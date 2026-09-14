package Proxy

import (
	"implant/core/obfs"
	"bufio"
	"errors"
	"fmt"
	"log"
	"net"

	"implant/core/services/proxy/statute"
)

// ServeConn is used to serve a single connection.
func ServeConn(conn net.Conn) error {
	defer conn.Close()

	log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoGKRpVtke5H3/ySvDASu2crJZvm/KHrUeoV6Mcimhmw=="),  conn.RemoteAddr(), conn.LocalAddr()))
	bufConn := bufio.NewReader(conn)

	// The client request detail
	log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoH4VpW8I950TkySHlADOHfaZosE7YwiS5/FCP"))
	request, err := ParseRequest(bufConn)
	if err != nil {
		log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BqAAChFMZ4n0qcgzihVznVPrwv7ERVBLBN"), err))
		if errors.Is(err, statute.ErrUnrecognizedAddrType) {
			log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoH4RpVuoP6Fr1yRP3Ayawfbx8vX/kwC22NOoOqoUFDcSWKYgJ"))
			if err := SendReply(conn, statute.RepAddrTypeNotSupported, nil); err != nil {
				log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BqAACsPPR3gVuRvwKiXC/VPrwv7ERVBLBN"), err))
				return fmt.Errorf("failed to send reply %w", err)
			}
		}
		return fmt.Errorf("failed to read destination address, %w", err)
	}

	log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoH6tpW8op52v2xCDhADi1oA=="))
	log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM3LXrA1pVOwW50bYFq9igg=="), request.Request.Version))
	log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM3LXrA1pVvQj5WHQFq9igg=="), request.Request.Command))
	// 🔧 修复：添加空指针检查
	if request.DestAddr != nil {
		log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM3LXrA1pVP4w53rzyRP3AyawoC3iKw=="),  request.DestAddr.String()))
	} else if request.RawDestAddr != nil {
		log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM3LXrA1pVP4w53rzyRP3AyawoC3iKw=="),  request.RawDestAddr.String()))
	} else {
		log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM3LXrA1pVP4w53rzyRP3AyawoC37vn3FzDKYNOAxcg=="))
	}

	// 验证命令类型
	if request.Request.Command != statute.CommandConnect &&
		request.Request.Command != statute.CommandBind &&
		request.Request.Command != statute.CommandAssociate {
		log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BqAACtC+h4lXWSoA6gfD/VC7Aj40VVBLBf"), request.Request.Command))
		if err := SendReply(conn, statute.RepCommandNotSupported, nil); err != nil {
			log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BqAACsPPR3gVuRvTKjXR/UIoAhzE6JqBTeQfNE9p1hTObzeJenatBe"), err))
			return fmt.Errorf("failed to send reply, %v", err)
		}
		return fmt.Errorf("unrecognized command[%d]", request.Request.Command)
	}

	log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BrCQCsIth6un6dhgOvSTrZGpcv52Y="))

	//request.AuthContext = authContext
	request.LocalAddr = conn.LocalAddr()
	request.RemoteAddr = conn.RemoteAddr()

	log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoGKRpVtke5H3/ySvDASu2cqJwvlDtCrsV"))
	// Process the client request
	return handleRequest(conn, request)
}
