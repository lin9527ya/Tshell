package config

// defaultBaiduBDUSS 是 HTTP 流量伪装用的占位 BDUSS cookie。
//
// 安全说明：此处不再硬编码任何疑似真实会话凭证。历史上此处曾内置一段疑似真实百度 BDUSS，
// 既存在凭证泄漏风险，又因所有部署共用同一串而成为可指纹化的静态特征。
// 生产环境应通过 per-listener 的 malleable C2 profile 覆盖该值（见阶段 B10），
// 不同监听器使用不同随机 cookie，避免静态指纹。
const defaultBaiduBDUSS = "PLACEHOLDER_BDUSS_REPLACE_VIA_LISTENER_PROFILE"

var (
	Http_get_metadata_prepend = "BDUSS=" + defaultBaiduBDUSS + "; BDUSS_BFESS=" + defaultBaiduBDUSS + ";SESSIONID=" // 每个http get 请求发送的数据前添加的字符串
	Http_get_output_prepend   = "{\"data\":{\"log_id\":\"3796460674\",\"action_rule\":{\"pos_1\":[\""               // 每个GET请求返回的数据在头部添加的字符串
	Http_get_output_append    = "%%\"],\"pos_2\":[],\"pos_3\":[]}}}"

	Http_post_id_prepend               = "BDUSS=" + defaultBaiduBDUSS + ";user="
	Http_post_id_append                = "%%; BDUSS_BFESS=" + defaultBaiduBDUSS
	Http_post_client_output_prepend    = "{\"data\":{\"log_id\":\"3796460677\",\"action_rule\":{\"pos_1\":[],\"pos_2\":[\""
	Http_post_client_output_append     = "%%\"],\"pos_3\":[]}}}"
	Http_post_client_output_type       = "print"
	Http_post_client_output_type_value = "_data"
	Http_post_server_output_prepend    = "{\"data\":{\"log_id\":\"3796460679\",\"action_rule\":{\"pos_1\":[],\"pos_2\":[],\"pos_3\":[\""
	Http_post_server_output_append     = "%%\"]}}}"
)

// ApplyHTTPConfig 将配置文件中的 HTTP masquerade（malleable C2 profile，config.yaml 的 http 段）
// 应用到本包级变量，供 communication/get.go、post.go 使用。空字段保留代码默认值（占位符），
// 避免空值破坏协议格式。由 InitGlobalConfig 启动时、ReloadConfigHandler 热重载时调用。
// 运维改 config.yaml 的 http 段即可调整伪装，无需重编译。
func ApplyHTTPConfig(cfg HTTPConfig) {
	if cfg.GetMetadataPrepend != "" {
		Http_get_metadata_prepend = cfg.GetMetadataPrepend
	}
	if cfg.GetOutputPrepend != "" {
		Http_get_output_prepend = cfg.GetOutputPrepend
	}
	if cfg.GetOutputAppend != "" {
		Http_get_output_append = cfg.GetOutputAppend
	}
	if cfg.PostIDPrepend != "" {
		Http_post_id_prepend = cfg.PostIDPrepend
	}
	if cfg.PostIDAppend != "" {
		Http_post_id_append = cfg.PostIDAppend
	}
	if cfg.PostClientOutputPrepend != "" {
		Http_post_client_output_prepend = cfg.PostClientOutputPrepend
	}
	if cfg.PostClientOutputAppend != "" {
		Http_post_client_output_append = cfg.PostClientOutputAppend
	}
	if cfg.PostClientOutputType != "" {
		Http_post_client_output_type = cfg.PostClientOutputType
	}
	if cfg.PostClientOutputTypeValue != "" {
		Http_post_client_output_type_value = cfg.PostClientOutputTypeValue
	}
	if cfg.PostServerOutputPrepend != "" {
		Http_post_server_output_prepend = cfg.PostServerOutputPrepend
	}
	if cfg.PostServerOutputAppend != "" {
		Http_post_server_output_append = cfg.PostServerOutputAppend
	}
}
