package Proxy

import (
	"implant/core/obfs"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"

	"implant/core/services/proxy/statute"
	"implant/core/utils"
)

// A Request represents request received by a server
type Request struct {
	statute.Request
	// AuthContext provided during negotiation
	// AuthContext *AuthContext
	// LocalAddr of the network server listen
	LocalAddr net.Addr
	// RemoteAddr of the network that sent the request
	RemoteAddr net.Addr
	// DestAddr of the actual destination (might be affected by rewrite)
	DestAddr *statute.AddrSpec
	// Reader connect of request
	Reader io.Reader
	// RawDestAddr of the desired destination
	RawDestAddr *statute.AddrSpec
}

// ParseRequest creates a new Request from the tcp connection
func ParseRequest(bufConn io.Reader) (*Request, error) {
	log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoH4VpVtke5H3/xCjkACWgyUKEE7JazDqMNM8jYieq"))
	hd, err := statute.ParseRequest(bufConn)
	if err != nil {
		log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BqAAChFMZ4n0onY8wMtY7YNboh6WOKgCTTZtubbCzy"), err))
		return nil, err
	}

	log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoH6tpW8I950TkyzTUACWsoA=="))
	log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM3LXrA1pVOwW50bYFq9igg=="), hd.Version))
	log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM3LXrA1pVvQj5WHQFq9igg=="), hd.Command))
	log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM3LXrA1pVvku5Ef0yz78AyW7oC3iPA=="), hd.DstAddr.AddrType))
	log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM3LXrA1pVP4w53rzyRP3AyawoC3iKw=="),  hd.DstAddr.String()))

	return &Request{
		Request:     hd,
		RawDestAddr: &hd.DstAddr,
		DestAddr:    &hd.DstAddr, // 🔧 修复：初始化DestAddr
		Reader:      bufConn,
	}, nil
}

// handleRequest is used for request processing after authentication
func handleRequest(write io.Writer, req *Request) error {
	// 🔧 修复：添加空指针检查
	var targetAddrStr string
	if req.RawDestAddr != nil {
		targetAddrStr = req.RawDestAddr.String()
	} else {
		targetAddrStr = "<未知地址>"
	}
	log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoGKRpVsEa5kryxCDwAAqyoC0iyVyLnzEG9xqNbO4fRrG7Sw+4OQ=="), req.Command, targetAddrStr))

	// Resolve the address if we have a FQDN
	dest := req.RawDestAddr
	if dest.FQDN != "" {
		log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoGK1pW8I950TkyRDYAyu9oC3iKw=="), dest.FQDN))
		addr, err := net.ResolveIPAddr("ip", dest.FQDN)
		if err != nil {
			log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BqAACsLPp7kVeciyyheCvVPrwv7ERVBLBN"), err))
			if err := SendReply(write, statute.RepHostUnreachable, nil); err != nil {
				log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BqAACsPPR3gVuQlDShegHUIoAi106HmiveQfNE9p1hTObzeJenatBe"), err))
				return fmt.Errorf("failed to send reply, %v", err)
			}
			return fmt.Errorf("failed to resolve destination[%v], %v", dest.FQDN, err)
		}
		dest.IP = addr.IP
		log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BrCQCsLPp7kVeciyyheCvWEp0i0n5VBLBI8lOfbCz3"),  dest.FQDN, dest.IP.String()))
	}

	// Apply any address rewrites
	req.DestAddr = req.RawDestAddr

	// Switch on the command
	switch req.Command {

	case statute.CommandConnect:
		log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoGLdpVsEa5kryb8AJqP5zzuhW5QXUgA=="))
		return handleConnect(write, req)

	case statute.CommandAssociate:
		log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoGLdpVsEa5krybdwUqfh521mCvXDSwC6f"))
		return handleAssociate(write, req)

	default:
		log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BqAACtC+h4lXWSoA6gfD/VC7Aj40VVBLBf"), req.Command))
		if err := SendReply(write, statute.RepCommandNotSupported, nil); err != nil {
			log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BqAACsPPR3gVuRvTKjXR/UIoAhzE6JqBTeQfNE9p1hTObzeJenatBe"), err))
			return fmt.Errorf("failed to send reply, %v", err)
		}
		return fmt.Errorf("unsupported command[%v]", req.Command)
	}
}

// handleConnect is used to handle a connect command
func handleConnect(writer io.Writer, request *Request) error {
	log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoGLdpVtke5H3/ySvDASu22UKJFqQscK8b9w0="),  request.DestAddr.String()))

	target, err := net.Dial("tcp", request.DestAddr.String())
	if err != nil {
		log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BqAAChDPt4j3+TtyGhRjzVPrwv7ERVBLBN"), err))
		msg := err.Error()
		resp := statute.RepHostUnreachable
		if strings.Contains(msg, "refused") {
			resp = statute.RepConnectionRefused
			log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoH4RpVuoP6Fr1xDDZADWVcq9svmr9wy6mN+0sqbMQ"))
		} else if strings.Contains(msg, "network is unreachable") {
			resp = statute.RepNetworkUnreachable
			log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoH4RpVuoP6Fr1yzLWAQCsfrVKvW7AzCuFN+0sqbMQ"))
		} else {
			log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoH4RpVuoP6Fr1yDf8ACeKfrVKvW7AzCuFN+0sqbMQ"))
		}
		if err := SendReply(writer, resp, nil); err != nil {
			log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BqAACsPPR3gVuduBavSRTVCYAi4nWKgCTTZtubbCzy"), err))
			return fmt.Errorf("failed to send reply, %v", err)
		}
		return fmt.Errorf("connect to %v failed, %v", request.RawDestAddr, err)
	}
	defer target.Close()

	log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BrCQChDPt4j3+TtyGhRjzWEp0i0n5VBLBI"),  target.RemoteAddr().String()))

	// Send success
	log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoH4RpVuoP6Fr1xDDZADWVfIVXvWvwwQa2N8Q1"))
	if err := SendReply(writer, statute.RepSuccess, target.LocalAddr()); err != nil {
		log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BqAACsPPR3gVuSpB+ibCTVCYAi4nWKgCTTZtubbCzy"), err))
		return fmt.Errorf("failed to send reply, %v", err)
	}

	log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoGKRpVtke5H3/yhr3ADaefrZkv3HpCrsV"))

	// Start proxying
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		buf := bufferPool.Get()
		defer bufferPool.Put(buf)
		log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoH4RpVtke5H3/xDLrAzShoC2kNIgKSuEb/0CBOGj2jzJv"))
		n, err := io.CopyBuffer(target, request.Reader, buf[:cap(buf)])
		log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoH4RpW9gy5FXlyzTUACavoC2kNIgKSuEb/0CBOGj2jzJv4BJ452LAPIEI5HonpnrNaNAO5A=="),  n, utils.RelayEndState(err)))
		target.Close() // client->target 结束:关闭 dst(=对端 src),解除 target->client 的 target.Read 阻塞
	}()
	go func() {
		defer wg.Done()
		buf := bufferPool.Get()
		defer bufferPool.Put(buf)
		log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoH4VpVtke5H3/xDLrAzShoC2zOZMIQeEb/0CBL2XtjTlv"))
		n, err := io.CopyBuffer(writer, target, buf[:cap(buf)])
		log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoH4VpW9gy5FXlyzTUACavoC2zOZMIQeEb/0CBL2XtjTlv4BJ452LAPIEI5HonpnrNaNAO5A=="),  n, utils.RelayEndState(err)))
		if c, ok := writer.(io.Closer); ok {
			c.Close()
		} // target->client 结束:关闭 stream(=对端 src)
	}()

	wg.Wait()
	log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BrCQAK/CvQRJkgySvDASu2f6NLvmn/"))

	return nil
}

// handleAssociate is used to handle a connect command
func handleAssociate(writer io.Writer, request *Request) error {
	log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoGLdpVtke5H3/ySvDASu2216UF6ImZcF+6F6EPw=="),  request.DestAddr.String()))

	bindLn, err := net.ListenUDP("udp", nil)
	if err != nil {
		log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BqAACsO/57umAhaN+gfSrVCqEi/FCHkDAB8lvX"), err))
		if err := SendReply(writer, statute.RepServerFailure, nil); err != nil {
			log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BqAACsPPR3gVuSsAKibBrVA6Ui/FCHkDDeQfNE9p1hTObzeJenatBe"), err))
			return fmt.Errorf("failed to send reply, %v", err)
		}
		return fmt.Errorf("listen udp failed, %v", err)
	}
	log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BrCQAc9zV5mkuRvCOifxPVEpYi41uJrAXeWOGbbCz3"),  request.DestAddr.String(), bindLn.LocalAddr().String()))
	log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoH6tpVss851LDyyToACevfJFcvX3fwQi76F6EPyWkDfmFJasYrW65U5NClFatY8IpftAO5A=="),  request.DestAddr.String()))

	// send BND.ADDR and BND.PORT, client used
	log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoH4RpVuoP6Fr1bdwUqfh521mCvmn/wR+kN+0sqbMQ"))
	if err = SendReply(writer, statute.RepSuccess, bindLn.LocalAddr()); err != nil {
		log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BqAACsPPR3gVuSpB+ibCTVCYAi4nWKgCTTZtubbCzy"), err))
		return fmt.Errorf("failed to send reply, %v", err)
	}

	go func() {
		// read from client and write to remote server
		conns := sync.Map{}
		bufPool := bufferPool.Get()
		defer func() {
			bufferPool.Put(bufPool)
			bindLn.Close()
			log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoGLpp5iHO5kHlyR/rAyKYf7p1vWTczQKW"))
			conns.Range(func(key, value any) bool {
				if connTarget, ok := value.(net.Conn); !ok {
					log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BtLM/xPEV2vkSSoiqhfhvVKokj4EyKiQ3eTtZI0ZdiW8LybYunatBejCaY"), key, value))
				} else {
					connTarget.Close()
					log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoGLppVuAt6E3ZecsXASCefK1AsF7xwhue6F6EPw=="), key))
				}
				return true
			})
		}()

		log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoGK1pVMgX5GTxecsXAC6AfIBpvW3qCrsV"))
		for {
			n, srcAddr, err := bindLn.ReadFromUDP(bufPool[:cap(bufPool)])
			if err != nil {
				if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
					log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoGLpp5iHO5kHlyR/rAyKYfKBkvVnXwRCIO+kM"))
					return
				}
				log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BqAAChHN57jkwhaN+hcwvWF6Mi1GSKgCTTZtubbCzy"), err))
				continue
			}
			log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoH4VpVfEo5FLEecsXAC6AfIBpvW3qHrUetpsM2+EOans7Kq84rE+4jCPLAg=="),  n, srcAddr.String()))

			pk, err := statute.ParseDatagram(bufPool[:n])
			if err != nil {
				log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BqAAChFMZ4n0ohaN+hcwvWF6Mh0kSKgCTTZtubbCzy"), err))
				continue
			}
			log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoH6tp5iHO50/EygLpADGVfZZpvkHoHrUeoQ=="),  pk.DstAddr.String()))

			// check src addr whether equal requst.DestAddr
			srcEqual := ((request.DestAddr.IP.IsUnspecified()) || request.DestAddr.IP.Equal(srcAddr.IP)) && (request.DestAddr.Port == 0 || request.DestAddr.Port == srcAddr.Port)
			if !srcEqual {
				log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BtLM/xPEV4u0qRsD+iezvZMIEv92CKgCTTZtubbO8Yd7GHVw+4OdkIU61wmFOYu3ra"),  request.DestAddr.String(), srcAddr.String()))
				continue
			}
			log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BrCQCvCfV7nWqRsQ+uTDfYNYwu2HuHmxI="))

			connKey := srcAddr.String() + "--" + pk.DstAddr.String()

			if target, ok := conns.Load(connKey); !ok {
				// if the 'connection' doesn't exist, create one and store it
				log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoGLdpVu0F5GHOyhn3ASG0z0mXv3rBwjW8OsE/qoch0nc+vw=="),  pk.DstAddr.String()))
				targetNew, err := net.Dial("udp", pk.DstAddr.String())
				if err != nil {
					log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BqAAChDPt4j38haN+gfRXWOooi/FCHkDAB8lvX"), err))
					continue
				}
				conns.Store(connKey, targetNew)
				log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BrCQAc9zV5mnSSjAivWSXWFKgi71OKrA7eacSbbCz3"), connKey))

				// read from remote server and write to original client
				go func() {
					bufPool := bufferPool.Get()
					defer func() {
						targetNew.Close()
						conns.Delete(connKey)
						bufferPool.Put(bufPool)
						log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoGLpp5iHO5kHayi/ADgSufINivVbdwRCIO+kMdimhmw=="), connKey))
					}()

					log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoGK1pVMgX5GTxecsXASCefK1AvXLiwS+v6F6EPw=="), connKey))
					for {
						buf := bufPool[:cap(bufPool)]
						n, err := targetNew.Read(buf)
						if err != nil {
							if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
								log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoGLpp5iHO5kHayi/ADgSufINivkzMwS2DN/sSpZ4p0nc+vw=="), connKey))
								return
							}
							log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BqAAChHN57jkwhaN+gfRXWOoohzVGJqTveds9J+Ky+yHJt"), err))
							return
						}
						log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoH4VpVfEo5FLEecsXASCefK1AvXLiwS+v6F6EKOwpf7+RTg=="), n))

						tmpBufPool := bufferPool.Get()
						proBuf := tmpBufPool
						proBuf = append(proBuf, pk.Header()...)
						proBuf = append(proBuf, buf[:n]...)
						log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoH4RpVuoP6Fr1ecsXAyi9f7dTvWnfwTuZNPYWq6Ir0nc+qNcw3R2iNA=="),  len(proBuf)))
						if _, err := bindLn.WriteTo(proBuf, srcAddr); err != nil {
							bufferPool.Put(tmpBufPool)
							log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BqAACsPPR3gVshaN+idTbVIJki/FCHkDAB8lvX"), err))
							return
						}
						bufferPool.Put(tmpBufPool)
						log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BrCQAc9zV7kleRlhuiaSrZGowh0HGKrgo="))
					}
				}()
				log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoH4RpVuoP6Fr1ecsXAC6AfIBpvWnfww6VNN4mdimhjLK2W9oXyA=="),  len(pk.Data)))
				if _, err := targetNew.Write(pk.Data); err != nil {
					log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BqAACsPPR3gVshaN+hcwvWF6Mi0FGIvzvdcvlE6LhsXPIh7Bfr"), err))
					return
				}
				log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BrCQAc9zV4lGqSoSGiaSrZGowh0HGKrgo="))
			} else {
				log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoGKRpV9gh5k7cywH3ACe5z0mXsF7xwhue6F6EPw=="), connKey))
				log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoH4RpVuoP6Fr1ecsXAC6AfIBpvWnfww6VNN4mdimhjLK2W9oXyA=="),  len(pk.Data)))
				if _, err := target.(net.Conn).Write(pk.Data); err != nil {
					log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BqAACsPPR3gVshaN+hcwvWF6Mi0FGIvzvdcvlE6LhsXPIh7Bfr"), err))
					return
				}
				log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BrCQAc9zV4lGqSoSGiaSrZGowh0HGKrgo="))
			}
		}
	}()

	buf := bufferPool.Get()
	defer bufferPool.Put(buf)

	log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoGK1pVMgX5GTxySHlADOHfaZovnfCwSm7OsE/qochxnk1"))
	for {
		_, err := request.Reader.Read(buf[:cap(buf)])
		if err != nil {
			bindLn.Close()
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BrCQAI4DbRQpM1eMqiQj/XCosi9m2JrAU="))
				return nil
			}
			log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BqAAAI4DbRQpM1eMqiQj/XCosi/FCHkDAB8lvX"), err))
			return err
		}
	}
}

// SendReply is used to send a reply message
// rep: reply status see statute's statute file
func SendReply(w io.Writer, rep uint8, bindAddr net.Addr) error {
	log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoH4RpVuoP6Fr1f8AEregFf55KvVv7HrXcWMhHzIi5zTM="), rep))

	rsp := statute.Reply{
		Version:  statute.VersionSocks5,
		Response: rep,
		BndAddr: statute.AddrSpec{
			AddrType: statute.ATYPIPv4,
			IP:       net.IPv4zero,
			Port:     0,
		},
	}

	if rsp.Response == statute.RepSuccess {
		if tcpAddr, ok := bindAddr.(*net.TCPAddr); ok && tcpAddr != nil {
			rsp.BndAddr.IP = tcpAddr.IP
			rsp.BndAddr.Port = tcpAddr.Port
			log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoH6tpVN4P5HTueMwXAyeAf5BHYsFKV68etg=="),  tcpAddr.IP.String(), tcpAddr.Port))
		} else if udpAddr, ok := bindAddr.(*net.UDPAddr); ok && udpAddr != nil {
			rsp.BndAddr.IP = udpAddr.IP
			rsp.BndAddr.Port = udpAddr.Port
			log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoH6tpVN4P5HTuecsXAyeAf5BHYsFKV68etg=="),  udpAddr.IP.String(), udpAddr.Port))
		} else {
			rsp.Response = statute.RepAddrTypeNotSupported
			log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BtLM/xPEV6uVeSuCChajrXAIkixFGKuRXcY8VE0oJrVNv/c5x73kzNJY4Ly176DOlPxHE="))
		}

		if rsp.BndAddr.IP.To4() != nil {
			rsp.BndAddr.AddrType = statute.ATYPIPv4
		} else if rsp.BndAddr.IP.To16() != nil {
			rsp.BndAddr.AddrType = statute.ATYPIPv6
		}
	}

	log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoH4RpVfke5mH8yRzKAwGkcqJhvmLqHrXcW/ZH0KW5zTM37NUX/BOoNz7LFeY9Y8MZoW2rcKd2hyaO1xj1"), rsp.Version, rsp.Response, rsp.BndAddr.AddrType))

	// Send the message
	_, err := w.Write(rsp.Bytes())
	if err != nil {
		log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BqAACsPPR3gVuRvwKiXC/VPrwv7ERVBLBN"), err))
	} else {
		log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BrCQCsIOh7u06Rox6uZjrWEp0i0n4="))
	}
	return err
}
