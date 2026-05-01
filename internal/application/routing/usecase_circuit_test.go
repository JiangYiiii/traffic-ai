package routing

import (
	"context"
	"errors"
	"testing"

	domainModel "github.com/trailyai/traffic-ai/internal/domain/model"
	domainRouting "github.com/trailyai/traffic-ai/internal/domain/routing"
	"github.com/trailyai/traffic-ai/internal/infrastructure/config"
	"github.com/trailyai/traffic-ai/pkg/crypto"
	"github.com/trailyai/traffic-ai/pkg/errcode"
)

// ---------- mock breaker ----------

type mockBreaker struct {
	allow map[int64]bool // 未配置的 accountID 默认允许
}

func (m *mockBreaker) Allow(_ context.Context, id int64) (bool, error) {
	if v, ok := m.allow[id]; ok {
		return v, nil
	}
	return true, nil
}
func (m *mockBreaker) RecordSuccess(context.Context, int64) error              { return nil }
func (m *mockBreaker) RecordFailure(context.Context, int64, string) error     { return nil }
func (m *mockBreaker) State(context.Context, int64) (string, error)           { return "closed", nil }

// ---------- minimal repo stubs ----------

type stubTokenGroupRepo struct {
	accountIDsByGroupName map[string][]int64
}

func (s *stubTokenGroupRepo) Create(context.Context, *domainRouting.TokenGroup) error { return nil }
func (s *stubTokenGroupRepo) FindByID(context.Context, int64) (*domainRouting.TokenGroup, error) {
	return nil, nil
}
func (s *stubTokenGroupRepo) FindByName(context.Context, string) (*domainRouting.TokenGroup, error) {
	return nil, nil
}
func (s *stubTokenGroupRepo) List(context.Context) ([]*domainRouting.TokenGroup, error) {
	return nil, nil
}
func (s *stubTokenGroupRepo) Update(context.Context, *domainRouting.TokenGroup) error { return nil }
func (s *stubTokenGroupRepo) Delete(context.Context, int64) error                    { return nil }
func (s *stubTokenGroupRepo) AddModelAccount(context.Context, int64, int64) error    { return nil }
func (s *stubTokenGroupRepo) RemoveModelAccount(context.Context, int64, int64) error { return nil }
func (s *stubTokenGroupRepo) ListModelAccountIDs(context.Context, int64) ([]int64, error) {
	return nil, nil
}
func (s *stubTokenGroupRepo) ListModelAccountIDsByName(_ context.Context, name string) ([]int64, error) {
	return s.accountIDsByGroupName[name], nil
}

type stubModelRepo struct {
	byName map[string]*domainModel.Model
	byID   map[int64]*domainModel.Model
}

func (s *stubModelRepo) Create(context.Context, *domainModel.Model) error { return nil }
func (s *stubModelRepo) FindByID(_ context.Context, id int64) (*domainModel.Model, error) {
	return s.byID[id], nil
}
func (s *stubModelRepo) FindByName(_ context.Context, n string) (*domainModel.Model, error) {
	return s.byName[n], nil
}
func (s *stubModelRepo) List(context.Context, domainModel.ListFilter) ([]*domainModel.Model, error) {
	return nil, nil
}
func (s *stubModelRepo) ListListedModels(context.Context) ([]*domainModel.Model, error) {
	return nil, nil
}
func (s *stubModelRepo) Update(context.Context, *domainModel.Model) error { return nil }
func (s *stubModelRepo) UpdateLastTest(context.Context, int64, bool, int, string) error {
	return nil
}
func (s *stubModelRepo) Delete(context.Context, int64) error { return nil }
func (s *stubModelRepo) ListByIDs(context.Context, []int64) ([]*domainModel.Model, error) {
	return nil, nil
}

type stubAccountRepo struct {
	activeByModelID map[int64][]*domainModel.ModelAccount
}

func (s *stubAccountRepo) Create(context.Context, *domainModel.ModelAccount) error { return nil }
func (s *stubAccountRepo) FindByID(context.Context, int64) (*domainModel.ModelAccount, error) {
	return nil, nil
}
func (s *stubAccountRepo) ListByModelID(context.Context, int64) ([]*domainModel.ModelAccount, error) {
	return nil, nil
}
func (s *stubAccountRepo) Update(context.Context, *domainModel.ModelAccount) error { return nil }
func (s *stubAccountRepo) Delete(context.Context, int64) error                     { return nil }
func (s *stubAccountRepo) ListActiveByModelIDs(_ context.Context, ids []int64) ([]*domainModel.ModelAccount, error) {
	var out []*domainModel.ModelAccount
	for _, id := range ids {
		out = append(out, s.activeByModelID[id]...)
	}
	return out, nil
}
func (s *stubAccountRepo) ListByIDs(_ context.Context, ids []int64) ([]*domainModel.ModelAccount, error) {
	index := make(map[int64]*domainModel.ModelAccount)
	for _, list := range s.activeByModelID {
		for _, a := range list {
			index[a.ID] = a
		}
	}
	var out []*domainModel.ModelAccount
	for _, id := range ids {
		if a, ok := index[id]; ok {
			out = append(out, a)
		}
	}
	return out, nil
}
func (s *stubAccountRepo) List(context.Context, domainModel.ModelAccountListFilter) ([]*domainModel.ModelAccount, error) {
	return nil, nil
}
func (s *stubAccountRepo) UpdateLastTest(context.Context, int64, bool, int, string) error { return nil }

// ---------- fixture builders ----------

// aesKey 必须是 32 字节（AES-256）。
var testAESKey = []byte("01234567890123456789012345678901")

func mustEncrypt(t *testing.T, plain string) string {
	t.Helper()
	enc, err := crypto.EncryptAES(plain, testAESKey)
	if err != nil {
		t.Fatalf("EncryptAES: %v", err)
	}
	return enc
}

func buildFixture(t *testing.T) (*UseCase, *stubTokenGroupRepo, *stubModelRepo, *stubAccountRepo, *mockBreaker) {
	t.Helper()
	m := &domainModel.Model{ID: 10, ModelName: "gpt-4o", IsActive: true, IsListed: true}
	tg := &stubTokenGroupRepo{accountIDsByGroupName: map[string][]int64{
		"default": {1, 2},
	}}
	mr := &stubModelRepo{
		byName: map[string]*domainModel.Model{"gpt-4o": m},
		byID:   map[int64]*domainModel.Model{10: m},
	}
	ar := &stubAccountRepo{activeByModelID: map[int64][]*domainModel.ModelAccount{
		10: {
			{ID: 1, ModelID: 10, Protocol: "openai", Weight: 1, IsActive: true, AuthType: "api_key", Credential: mustEncrypt(t, "sk-aaa")},
			{ID: 2, ModelID: 10, Protocol: "openai", Weight: 1, IsActive: true, AuthType: "api_key", Credential: mustEncrypt(t, "sk-bbb")},
		},
	}}
	br := &mockBreaker{allow: map[int64]bool{}}
	uc := NewUseCase(tg, mr, ar, testAESKey, config.OAuthConfig{}, br)
	return uc, tg, mr, ar, br
}

// ---------- tests ----------

// 注意：UseCase.SelectModelAccount 会原地把 Credential 解密覆盖掉密文，
// 因此测试每次迭代都需要重建 fixture，否则下一次解密会失败。

func TestSelectModelAccount_SkipsOpenAccount(t *testing.T) {
	for i := 0; i < 10; i++ {
		uc, _, _, _, br := buildFixture(t)
		br.allow[1] = false // 账号 1 熔断
		res, err := uc.SelectModelAccount(context.Background(), "default", "gpt-4o", "openai")
		if err != nil {
			t.Fatalf("SelectModelAccount err: %v", err)
		}
		if res.Account.ID != 2 {
			t.Fatalf("iter %d: expected account 2, got %d", i, res.Account.ID)
		}
	}
}

func TestSelectModelAccountExcluding_SkipsExcludedIDs(t *testing.T) {
	for i := 0; i < 10; i++ {
		uc, _, _, _, _ := buildFixture(t)
		res, err := uc.SelectModelAccountExcluding(context.Background(), "default", "gpt-4o", "openai", []int64{1})
		if err != nil {
			t.Fatalf("SelectModelAccountExcluding err: %v", err)
		}
		if res.Account.ID != 2 {
			t.Fatalf("iter %d: expected account 2 after excluding 1, got %d", i, res.Account.ID)
		}
	}
}

func TestSelectModelAccount_AllFilteredReturnsNoRoute(t *testing.T) {
	uc, _, _, _, br := buildFixture(t)
	br.allow[1] = false
	br.allow[2] = false
	_, err := uc.SelectModelAccount(context.Background(), "default", "gpt-4o", "openai")
	if !errors.Is(err, errcode.ErrNoAvailableRoute) {
		t.Fatalf("expected ErrNoAvailableRoute, got %v", err)
	}
}

func TestSelectOpenAICompatibleAccount_WithHint(t *testing.T) {
	uc, _, _, _, _ := buildFixture(t)
	res, err := uc.SelectOpenAICompatibleAccount(context.Background(), "default", "gpt-4o")
	if err != nil {
		t.Fatal(err)
	}
	if res.Account.Credential == "" {
		t.Fatal("expected decrypted credential")
	}
}

func TestSelectOpenAICompatibleAccount_NoHint(t *testing.T) {
	uc, _, _, _, _ := buildFixture(t)
	res, err := uc.SelectOpenAICompatibleAccount(context.Background(), "default", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Model.ModelName != "gpt-4o" {
		t.Fatalf("model: %s", res.Model.ModelName)
	}
	if res.Account.ID != 1 && res.Account.ID != 2 {
		t.Fatalf("unexpected account id %d", res.Account.ID)
	}
}
