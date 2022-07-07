# Crow protocal

The crow protocol is a proxy protocol built on TCP multiplexing technology. It eliminates some common characteristics of proxy software, such as frequent connection establishment and disconnection when browsing websites. It makes it more difficult to be detected by firewalls.

When the client is initialized, it needs to establish and maintain a connection with the server. After that, the client and server communicate through the following commands.

# CMD1

The server and client can request each other to send random data of a specified size and simply discarded. The purpose of this command is to shape traffic to avoid being identified.

+-----+-----+
|  1  | Len |
+-----+-----+
|  1  |  2  |
+-----+-----+

The other one replys

+-----+-----+-----+
| Rsv | Len | Msg |
+-----+-----+-----+
|  1  |  2  |  N  |
+-----+-----+-----+

# CMD 2

Write

+-----+-----+-----+-----+
| Rsv | ID  | Len | Msg |
+-----+-----+-----+-----+
|  1  |  2  |  2  |  N  |
+-----+-----+-----+-----+

# CMD 3

Client wishes to establish a connection.

+-----+-----+-----+
| Net | Len | Dst |
+-----+-----+-----+
|  1  |  2  |  N  |
+-----+-----+-----+

Net:
     1 TCP
     3 UDP

+-----+-----+
| Rep | ID  |
+-----+-----+
|  1  |  2  |
+-----+-----+

Rep: Reply field
     0 succeeded
     1 general failure

# CMD 4: close a file descriptor

+-----+-----+
| Rsv | ID  |
+-----+-----+
|  1  |  2  |
+-----+-----+

func (s *Server) ServeCmd(ctx *daze.Context, cli io.ReadWriteCloser, c chan<- []byte) {
	var (
		buf          []byte
		headerDstLen uint8
		headerMsgLen uint16
		err          error
	)
	for {
		buf = make([]byte, 2048)
		_, err = io.ReadFull(cli, buf[:8])
		if err != nil {
			break
		}
		log.Printf("%s server recv=0x%s", ctx.Cid, hex.EncodeToString(buf[:8]))
		switch buf[0] {
		case 1:
			c <- buf
		case 2:
			headerMsgLen = binary.BigEndian.Uint16(buf[3:5])
			_, err = io.ReadFull(cli, buf[8:8+headerMsgLen])
			if err == nil {
				c <- buf
			}
		case 3:
			headerDstLen = buf[4]
			_, err = io.ReadFull(cli, buf[8:8+headerDstLen])
			if err == nil {
				c <- buf
			}
		case 4:
			c <- buf
		}
	}
	buf = make([]byte, 2048)
	buf[0] = 0
	c <- buf
	close(c)
}
