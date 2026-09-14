package oss

import (
	"BackendTemplate/pkg/encrypt"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"io"
	"strings"
	"BackendTemplate/pkg/utils/logx"
)

type Client struct {
	Cli             *oss.Client
	Bucket          *oss.Bucket
	Endpoint        string
	AccessKeyId     string
	AccessKeySecret string
	BucketName      string
}

var Service *Client

// var c *oss.Bucket

func InitClient(endPoint, accessKeyId, accessKeySecret, bucketName string) error {
	var ossClient *oss.Client
	var err error

	ossClient, err = oss.New(endPoint, accessKeyId, accessKeySecret)
	if err != nil {
		return err
	}

	var ossBucket *oss.Bucket
	ossBucket, err = ossClient.Bucket(bucketName)
	if err != nil {
		return err
	}

	Service = &Client{
		Cli:             ossClient,
		Bucket:          ossBucket,
		Endpoint:        endPoint,
		AccessKeyId:     accessKeyId,
		AccessKeySecret: accessKeySecret,
		BucketName:      bucketName,
	}
	return nil
}
func List(c *Client) []oss.ObjectProperties {

	lsRes, err := c.Bucket.ListObjects(oss.MaxKeys(3), oss.Prefix(""))
	if err != nil {
		// 错误返回空列表，不让单次 OSS 故障杀死整个 C2 进程
		logx.Println("Error:", err)
		return nil
	}
	//logx.Println(lsRes)
	return lsRes.Objects

}
func Send(c *Client, name string, content []byte) {
	encodeData, err := encrypt.EncodeBase64(content)
	if err != nil {
		logx.Println("[-] 编码失败:", err)
		return
	}
	// 1.通过字符串上传对象
	f := strings.NewReader(string(encodeData))
	err = c.Bucket.PutObject(name, f)
	if err != nil {
		logx.Println("[-]", "上传失败:", err)
		return
	}

}
func Get(c *Client, name string) []byte {
	//logx.Println(name)
	body, err := c.Bucket.GetObject(name)
	if err != nil {
		return nil
	}
	// 数据读取完成后，获取的流必须关闭，否则会造成连接泄漏，导致请求无连接可用，程序无法正常工作。
	defer body.Close()
	// println(body)
	data, err := io.ReadAll(body)
	if err != nil {
		logx.Println("Error:", err)
		return nil
	}
	//logx.Println(string(data))
	decodeData, err := encrypt.DecodeBase64(data)
	if err != nil {
		logx.Println("Error:", err)
		return nil
	}
	return decodeData
}

func Del(c *Client, name string) {
	err := c.Bucket.DeleteObject(name)
	if err != nil {
		// 删除失败仅记录日志，不 panic
		logx.Println("[-] 删除失败:", name, err)
	}

}
