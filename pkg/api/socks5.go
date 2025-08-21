package api

import (
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/proxy"
	"BackendTemplate/pkg/sendcommand"
	"fmt"
	"github.com/gin-gonic/gin"
	"net"
	"net/http"
	"strings"
)

func Socks5List(c *gin.Context) {
	var socks5Body struct {
		Uid string `form:"uid"`
	}
	if err := c.ShouldBindQuery(&socks5Body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}
	var socks5 []database.Socks5
	database.Engine.Where("uid = ?", socks5Body.Uid).Find(&socks5)
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": socks5})
}
func Socks5Start(c *gin.Context) {
	var socks5Body struct {
		Uid            string `json:"uid"`
		ConnectAddress string `json:"ConnectAddress"`
		Socks5port     string `json:"Socks5port"`
		UserName       string `json:"UserName"`
		Password       string `json:"Password"`
	}
	if err := c.ShouldBindJSON(&socks5Body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}
	listenPort := strings.Split(socks5Body.ConnectAddress, ":")[1]
	inUse, err := isPortInUse(listenPort)
	if err != nil {
		fmt.Printf("检测端口 %s 时发生错误: %v\n", listenPort, err)
	}
	if inUse {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": listenPort + "端口被占用"})
	}
	inUse, err = isPortInUse(socks5Body.Socks5port)
	if err != nil {
		fmt.Printf("检测端口 %s 时发生错误: %v\n", socks5Body.Socks5port, err)
	}
	if inUse {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": socks5Body.Socks5port + "端口被占用"})
	}

	database.Engine.Insert(&database.Socks5{Type: "socks5", Uid: socks5Body.Uid, ConnectAddress: socks5Body.ConnectAddress, Socks5port: socks5Body.Socks5port, UserName: socks5Body.UserName, Password: socks5Body.Password, Status: 1})

	go proxy.ReverseSocksServer(":"+listenPort, "0.0.0.0:"+socks5Body.Socks5port, "psk", "", "", socks5Body.UserName, socks5Body.Password)
	sendcommand.SendCommand(socks5Body.Uid, "socks5 "+socks5Body.ConnectAddress)
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": "socks5 started"})
}
func Socks5Open(c *gin.Context) {
	var socks5Body struct {
		Uid            string `json:"uid"`
		ConnectAddress string `json:"ConnectAddress"`
		Socks5port     string `json:"Socks5port"`
		UserName       string `json:"UserName"`
		Password       string `json:"Password"`
	}
	if err := c.ShouldBindJSON(&socks5Body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}
	listenPort := strings.Split(socks5Body.ConnectAddress, ":")[1]
	inUse, err := isPortInUse(listenPort)
	if err != nil {
		fmt.Printf("检测端口 %s 时发生错误: %v\n", listenPort, err)
	}
	if inUse {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": listenPort + "端口被占用"})
	}
	inUse, err = isPortInUse(socks5Body.Socks5port)
	if err != nil {
		fmt.Printf("检测端口 %s 时发生错误: %v\n", socks5Body.Socks5port, err)
	}
	if inUse {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": socks5Body.Socks5port + "端口被占用"})
	}
	database.Engine.Where("uid = ? AND connect_address = ? AND socks5port = ? AND user_name = ? AND password = ?", socks5Body.Uid, socks5Body.ConnectAddress, socks5Body.Socks5port, socks5Body.UserName, socks5Body.Password).Update(&database.Socks5{Status: 1})

	go proxy.ReverseSocksServer(":"+listenPort, "0.0.0.0:"+socks5Body.Socks5port, "psk", "", "", socks5Body.UserName, socks5Body.Password)
	sendcommand.SendCommand(socks5Body.Uid, "socks5 "+socks5Body.ConnectAddress)
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": "socks5 started"})
}
func Socks5Close(c *gin.Context) {
	var socks5Body struct {
		Uid            string `json:"uid"`
		ConnectAddress string `json:"ConnectAddress"`
		Socks5port     string `json:"Socks5port"`
		UserName       string `json:"UserName"`
		Password       string `json:"Password"`
	}
	if err := c.ShouldBindJSON(&socks5Body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}
	listenPort := strings.Split(socks5Body.ConnectAddress, ":")[1]
	database.Engine.Where("uid = ? AND connect_address = ? AND socks5port = ? AND user_name = ? AND password = ?", socks5Body.Uid, socks5Body.ConnectAddress, socks5Body.Socks5port, socks5Body.UserName, socks5Body.Password).Update(&database.Socks5{Status: 2})

	if _, exists := proxy.Socks5Server[":"+listenPort]; exists {
		err := proxy.Socks5Server[":"+listenPort].Close()
		proxy.MuSocks5Server.Lock()
		delete(proxy.Socks5Server, ":"+listenPort)
		proxy.MuSocks5Server.Unlock()
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"status": 400, "data": "Socks5 closed failed"})
			return
		}
	}
	if _, exists := proxy.Socks5Server["0.0.0.0:"+socks5Body.Socks5port]; exists {
		err := proxy.Socks5Server["0.0.0.0:"+socks5Body.Socks5port].Close()
		proxy.MuSocks5Server.Lock()
		delete(proxy.Socks5Server, "0.0.0.0:"+socks5Body.Socks5port)
		proxy.MuSocks5Server.Unlock()
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"status": 400, "data": "Socks5 closed failed"})
			return
		}
	}

	sendcommand.SendCommand(socks5Body.Uid, "socks5Close")
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": "socks5 closed"})
}
func Socks5Delete(c *gin.Context) {
	var socks5Body struct {
		Uid            string `json:"uid"`
		ConnectAddress string `json:"ConnectAddress"`
		Socks5port     string `json:"Socks5port"`
		UserName       string `json:"UserName"`
		Password       string `json:"Password"`
	}
	if err := c.ShouldBindJSON(&socks5Body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}
	listenPort := strings.Split(socks5Body.ConnectAddress, ":")[1]
	var s database.Socks5
	database.Engine.Where("uid = ? AND connect_address = ? AND socks5port = ? AND user_name = ? AND password = ?", socks5Body.Uid, socks5Body.ConnectAddress, socks5Body.Socks5port, socks5Body.UserName, socks5Body.Password).Get(&s)
	if s.Status == 1 {
		if _, exists := proxy.Socks5Server[":"+listenPort]; exists {
			err := proxy.Socks5Server[":"+listenPort].Close()
			proxy.MuSocks5Server.Lock()
			delete(proxy.Socks5Server, ":"+listenPort)
			proxy.MuSocks5Server.Unlock()
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"status": 400, "data": "Socks5 closed failed"})
				return
			}
		}
		if _, exists := proxy.Socks5Server["0.0.0.0:"+socks5Body.Socks5port]; exists {
			err := proxy.Socks5Server["0.0.0.0:"+socks5Body.Socks5port].Close()
			proxy.MuSocks5Server.Lock()
			delete(proxy.Socks5Server, "0.0.0.0:"+socks5Body.Socks5port)
			proxy.MuSocks5Server.Unlock()
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"status": 400, "data": "Socks5 closed failed"})
				return
			}
		}
		sendcommand.SendCommand(socks5Body.Uid, "socks5Close")
	}
	database.Engine.Where("uid = ? AND connect_address = ? AND socks5port = ? AND user_name = ? AND password = ?", socks5Body.Uid, socks5Body.ConnectAddress, socks5Body.Socks5port, socks5Body.UserName, socks5Body.Password).Delete(&database.Socks5{})
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": "socks5 deleted"})
}

// isPortInUse 检测指定端口是否被占用
func isPortInUse(port string) (bool, error) {
	// 尝试监听该端口
	listener, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		// 如果监听失败，判断是否是端口被占用
		if opErr, ok := err.(*net.OpError); ok {
			if opErr.Err.Error() == "bind: address already in use" ||
				opErr.Err.Error() == "listen tcp :"+fmt.Sprintf("%s", port)+": bind: Only one usage of each socket address (protocol/network address/port) is normally permitted." {
				return true, nil // 端口被占用
			}
		}
		return false, err // 其他错误
	}

	// 如果监听成功，关闭 listener 并返回未占用
	_ = listener.Close()
	return false, nil
}
