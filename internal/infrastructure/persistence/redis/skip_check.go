package redis

import "os"

// SkipCheck 是否跳过 Redis 启动 ping 与 /readyz 中的 Redis 检查。
// 临时排障开关：设置 TRAFFIC_REDIS_SKIP_CHECK=1，排查完成后务必关闭。
func SkipCheck() bool {
	switch os.Getenv("TRAFFIC_REDIS_SKIP_CHECK") {
	case "1", "true", "TRUE", "True", "yes", "YES":
		return true
	default:
		return false
	}
}
