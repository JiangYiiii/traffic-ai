package routing

import (
	"os"
	"testing"

	"github.com/trailyai/traffic-ai/pkg/logger"
)

// 初始化 zap logger，避免 usecase 内部 logger.L.Errorw 触发 nil pointer。
func TestMain(m *testing.M) {
	logger.Init("error", "json", "stdout", "")
	os.Exit(m.Run())
}
