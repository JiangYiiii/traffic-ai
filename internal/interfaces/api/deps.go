// Package api 控制面共享依赖装配（用户平面 / 管理平面共用同一套 UseCase 与 Handler）。
package api

import (
	"database/sql"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	authuc "github.com/trailyai/traffic-ai/internal/application/auth"
	appbilling "github.com/trailyai/traffic-ai/internal/application/billing"
	appmodel "github.com/trailyai/traffic-ai/internal/application/model"
	appmonitor "github.com/trailyai/traffic-ai/internal/application/monitor"
	appoauth "github.com/trailyai/traffic-ai/internal/application/oauth"
	appratelimit "github.com/trailyai/traffic-ai/internal/application/ratelimit"
	approuting "github.com/trailyai/traffic-ai/internal/application/routing"
	apptoken "github.com/trailyai/traffic-ai/internal/application/token"
	ratelimitDomain "github.com/trailyai/traffic-ai/internal/domain/ratelimit"
	"github.com/trailyai/traffic-ai/internal/infrastructure/config"
	"github.com/trailyai/traffic-ai/internal/infrastructure/persistence/mysql"
	redispkg "github.com/trailyai/traffic-ai/internal/infrastructure/persistence/redis"
	"github.com/trailyai/traffic-ai/internal/interfaces/api/handler"
	"github.com/trailyai/traffic-ai/internal/interfaces/api/middleware"
	"github.com/trailyai/traffic-ai/pkg/jwt"
)

// controlPlane 用户平面与管理平面共用的 Handler 与中间件依赖。
type controlPlane struct {
	cfg *config.Config

	jwtMgr *jwt.Manager

	authHandler    *handler.AuthHandler
	tokenHandler   *handler.TokenHandler
	billingHandler *handler.BillingHandler
	modelHandler   *handler.ModelHandler
	rlHandler      *handler.RateLimitHandler
	accountHandler *handler.AccountHandler
	oauthHandler   *handler.OAuthHandler
	monitorHandler *handler.MonitorHandler
}

func newControlPlane(cfg *config.Config, db *sql.DB, rdb *redis.Client) *controlPlane {
	if cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	jwtMgr := jwt.NewManager(cfg.JWT.Secret, cfg.JWT.AccessTTL, cfg.JWT.RefreshTTL)
	aesKey := []byte(cfg.Crypto.AESKey)

	userRepo := mysql.NewUserRepo(db)
	tokenRepo := mysql.NewTokenRepo(db)
	balanceRepo := mysql.NewBalanceRepo(db)
	balanceLogRepo := mysql.NewBalanceLogRepo(db)
	redeemRepo := mysql.NewRedeemCodeRepo(db)
	modelRepo := mysql.NewModelRepo(db)
	modelAccountRepo := mysql.NewModelAccountRepo(db)
	tokenGroupRepo := mysql.NewTokenGroupRepo(db)
	rlRuleRepo := mysql.NewRateLimitRuleRepo(db)
	usageLogRepo := mysql.NewUsageLogRepo(db)

	monitorRepo := mysql.NewMonitorRepo(db)
	oauthStateRepo := mysql.NewOAuthStateRepo(db)

	codeStore := redispkg.NewVerifyCodeStore(rdb)
	lockStore := redispkg.NewLoginLockStore(rdb)
	balanceCache := redispkg.NewBalanceCache(rdb)
	monitorCounter := redispkg.NewMonitorCounter(rdb)

	billingUC := appbilling.NewUseCase(db, balanceRepo, balanceLogRepo, redeemRepo, balanceCache)
	authUC := authuc.NewUseCase(userRepo, codeStore, lockStore, jwtMgr, billingUC)
	tokenUC := apptoken.NewUseCase(tokenRepo)
	modelUC := appmodel.NewUseCase(modelRepo, modelAccountRepo, aesKey)
	// control 面不参与数据面熔断决策，传 nil 即可（选号走全量候选）。
	routingUC := approuting.NewUseCase(tokenGroupRepo, modelRepo, modelAccountRepo, aesKey, cfg.OAuth, nil)

	oauthUC := appoauth.NewUseCase(cfg.OAuth, oauthStateRepo, aesKey)
	monitorUC := appmonitor.NewUseCase(monitorRepo, modelRepo, modelAccountRepo, monitorCounter)

	var rlUC *appratelimit.UseCase
	rlLimiter := redispkg.NewRedisRateLimiter(rdb, func() []*ratelimitDomain.RateLimitRule {
		return rlUC.ActiveRules()
	})
	rlUC = appratelimit.NewUseCase(rlRuleRepo, rlLimiter)

	return &controlPlane{
		cfg: cfg,

		jwtMgr: jwtMgr,

		authHandler:    handler.NewAuthHandler(authUC),
		tokenHandler:   handler.NewTokenHandler(tokenUC),
		billingHandler: handler.NewBillingHandler(billingUC, userRepo),
		modelHandler:   handler.NewModelHandler(modelUC, routingUC, usageLogRepo),
		rlHandler:      handler.NewRateLimitHandler(rlUC),
		accountHandler: handler.NewAccountHandler(userRepo, balanceRepo, tokenRepo, usageLogRepo),
		oauthHandler:   handler.NewOAuthHandler(oauthUC),
		monitorHandler: handler.NewMonitorHandler(monitorUC),
	}
}

func (p *controlPlane) registerUserAPI(r *gin.Engine) {
	p.authHandler.Register(r.Group("/auth"))

	accountGroup := r.Group("/account")
	accountGroup.Use(middleware.JWTAuth(p.jwtMgr))
	p.accountHandler.Register(accountGroup)

	meGroup := r.Group("/me")
	meGroup.Use(middleware.JWTAuth(p.jwtMgr))
	p.tokenHandler.Register(meGroup)
	p.billingHandler.RegisterUser(meGroup)
	p.modelHandler.RegisterUser(meGroup)
}

func (p *controlPlane) registerAdminAPI(r *gin.Engine) {
	p.authHandler.Register(r.Group("/auth"))

	accountGroup := r.Group("/account")
	accountGroup.Use(middleware.JWTAuth(p.jwtMgr))
	p.accountHandler.Register(accountGroup)

	// 客户管理端 —— admin 或 super_admin 均可访问
	customerGroup := r.Group("/admin")
	customerGroup.Use(middleware.JWTAuth(p.jwtMgr))
	customerGroup.Use(middleware.RequireAdmin())
	p.billingHandler.RegisterAdmin(customerGroup)

	// 模型管理端 + 监控端 —— 仅 super_admin 可访问
	modelMgmtGroup := r.Group("/admin")
	modelMgmtGroup.Use(middleware.JWTAuth(p.jwtMgr))
	modelMgmtGroup.Use(middleware.RequireSuperAdmin())
	p.modelHandler.Register(modelMgmtGroup)
	p.rlHandler.Register(modelMgmtGroup)
	p.oauthHandler.RegisterStart(modelMgmtGroup)
	p.monitorHandler.Register(modelMgmtGroup)

	// OAuth callback 不需要认证（浏览器直接跳转，不带 Authorization header）
	p.oauthHandler.RegisterCallback(r)
}
