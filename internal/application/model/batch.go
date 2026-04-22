package model

import (
	"context"
	"strings"

	domain "github.com/trailyai/traffic-ai/internal/domain/model"
	"github.com/trailyai/traffic-ai/pkg/errcode"
)

// BatchCreateFailure 批量创建中单条失败。
type BatchCreateFailure struct {
	Index     int
	ModelName string
	Message   string
}

// BatchCreateOutcome 批量创建结果。
type BatchCreateOutcome struct {
	Created []*domain.Model
	Failed  []BatchCreateFailure
}

// BatchModelItem 单条批量创建入参。
type BatchModelItem struct {
	Model *domain.Model
	Opts  *CreateModelOpts
}

// BatchCreateModels 逐项调用 CreateModel；单条失败不中断。
func (uc *UseCase) BatchCreateModels(ctx context.Context, items []BatchModelItem) BatchCreateOutcome {
	out := BatchCreateOutcome{
		Created: make([]*domain.Model, 0),
		Failed:  make([]BatchCreateFailure, 0),
	}
	for i, it := range items {
		if err := uc.CreateModel(ctx, it.Model, it.Opts); err != nil {
			msg := err.Error()
			if ae, ok := err.(*errcode.AppError); ok {
				msg = ae.Message
			}
			name := ""
			if it.Model != nil {
				name = it.Model.ModelName
			}
			out.Failed = append(out.Failed, BatchCreateFailure{Index: i, ModelName: name, Message: msg})
			continue
		}
		m := it.Model
		full, err := uc.modelRepo.FindByID(ctx, m.ID)
		if err != nil || full == nil {
			out.Created = append(out.Created, m)
			continue
		}
		out.Created = append(out.Created, full)
	}
	return out
}

// BatchOptsFromFields 由 DTO 字段构造可选账号参数（仅当密钥非空）。
func BatchOptsFromFields(credential, endpoint string) *CreateModelOpts {
	credential = strings.TrimSpace(credential)
	if credential == "" {
		return nil
	}
	return &CreateModelOpts{
		AccountCredential: credential,
		AccountEndpoint:   strings.TrimSpace(endpoint),
	}
}
