package proxy

import (
	"log"
	"net"

	"BackendTemplate/pkg/proxy/statute"
)

// AuthContext A Request encapsulates authentication state provided
// during negotiation
type AuthContext struct {
	// Provided auth method
	Method uint8
	// Payload provided during negotiation.
	// Keys depend on the used auth method.
	// For UserPass auth contains username/password
	// Payload map[string]string
}

// Authenticator provide auth
type Authenticator interface {
	Authenticate(conn net.Conn) error
	GetCode() uint8
}

// NoAuthAuthenticator is used to handle the "No Authentication" mode
type NoAuthAuthenticator struct{}

// GetCode implement interface Authenticator
func (a NoAuthAuthenticator) GetCode() uint8 {
	log.Printf("[SOCKS5-AUTH] 🔓 NoAuth认证模式 - 返回方法代码: %d", statute.MethodNoAuth)
	return statute.MethodNoAuth
}

// Authenticate implement interface Authenticator
func (a NoAuthAuthenticator) Authenticate(conn net.Conn) error {
	log.Printf("[SOCKS5-AUTH] 🔓 NoAuth认证 - 向客户端发送认证响应: Version=%d, Method=%d", statute.VersionSocks5, statute.MethodNoAuth)
	_, err := conn.Write([]byte{statute.VersionSocks5, statute.MethodNoAuth})
	if err != nil {
		log.Printf("[SOCKS5-AUTH] ❌ NoAuth认证失败 - 发送响应错误: %v", err)
	} else {
		log.Printf("[SOCKS5-AUTH] ✅ NoAuth认证成功")
	}
	return err
}

// UserPassAuthenticator is used to handle username/password based
// authentication
type UserPassAuthenticator struct {
	Username string
	Password string
}

// GetCode implement interface Authenticator
func (a UserPassAuthenticator) GetCode() uint8 {
	log.Printf("[SOCKS5-AUTH] 🔐 UserPass认证模式 - 返回方法代码: %d", statute.MethodUserPassAuth)
	return statute.MethodUserPassAuth
}

// Authenticate implement interface Authenticator
func (a UserPassAuthenticator) Authenticate(conn net.Conn) error {
	log.Printf("[SOCKS5-AUTH] 🔐 UserPass认证开始 - 期望用户名: %s", a.Username)

	// reply the client to use user/pass auth
	log.Printf("[SOCKS5-AUTH] 📤 发送UserPass认证方法选择 - Version=%d, Method=%d", statute.VersionSocks5, statute.MethodUserPassAuth)
	if _, err := conn.Write([]byte{statute.VersionSocks5, statute.MethodUserPassAuth}); err != nil {
		log.Printf("[SOCKS5-AUTH] ❌ 发送认证方法失败: %v", err)
		return err
	}

	// get user and user's password
	log.Printf("[SOCKS5-AUTH] 📥 等待接收用户名密码...")
	nup, err := statute.ParseUserPassRequest(conn)
	if err != nil {
		log.Printf("[SOCKS5-AUTH] ❌ 解析用户名密码请求失败: %v", err)
		return err
	}

	log.Printf("[SOCKS5-AUTH] 📥 收到认证信息 - 用户名: %s, 密码长度: %d", string(nup.User), len(nup.Pass))

	// Verify the password
	if a.Username == string(nup.User) && a.Password == string(nup.Pass) {
		log.Printf("[SOCKS5-AUTH] ✅ 用户名密码验证成功")
		if _, err = conn.Write([]byte{statute.UserPassAuthVersion, statute.AuthSuccess}); err != nil {
			log.Printf("[SOCKS5-AUTH] ❌ 发送认证成功响应失败: %v", err)
			return err
		}
		log.Printf("[SOCKS5-AUTH] 📤 已发送认证成功响应")
		return nil
	}

	log.Printf("[SOCKS5-AUTH] ❌ 用户名密码验证失败 - 提供的用户名: %s, 期望: %s", string(nup.User), a.Username)
	if _, err := conn.Write([]byte{statute.UserPassAuthVersion, statute.AuthFailure}); err != nil {
		log.Printf("[SOCKS5-AUTH] ❌ 发送认证失败响应失败: %v", err)
		return err
	}
	log.Printf("[SOCKS5-AUTH] 📤 已发送认证失败响应")
	return statute.ErrUserAuthFailed
}
