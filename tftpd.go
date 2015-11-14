package main

import "os"
import "net"
import "fmt"

func DieOnError(err error) {
	if err != nil {
		fmt.Println("Error: ", err)
		os.Exit(0)
	}
}

/* func insertBlockNum(ack []byte, num int) {
	//    err := binary.Write(ack, binary.LittleEndian, num)
	n := PutUvarint(ack, num)
	if err != nil {
		fmt.Println("binary.Write failed:", err)
	}
	fmt.Printf("% x", ack.Bytes())
}
*/ 
func sendAck(conn *net.UDPConn, addr *net.UDPAddr,blocknum []byte) {
	var ack = "\x00\x04"
//	insertBlockNum(ack[2], blocknum)
	var res = append([]byte(ack),blocknum...)
	fmt.Println(res)
	_, err := conn.WriteToUDP(res, addr)
	if err != nil {
		fmt.Printf("Couldn't send response %v", err)
	}
}

func main() {
	port := os.Args[1]
	fmt.Println("listening on port " + port)

	ServerAddr, err := net.ResolveUDPAddr("udp", ":"+port)
	DieOnError(err)

	/* Now listen at selected port */
	ServerConn, err := net.ListenUDP("udp", ServerAddr)
	DieOnError(err)
	defer ServerConn.Close()

	buf := make([]byte, 1024)

//	nil blocknum
    var blocknum = "\x00\x00"

	for {
		n, raddr, err := ServerConn.ReadFromUDP(buf)
		fmt.Println("OpCode",buf[0:2], "BlockNum", blocknum)		
		fmt.Println("Received ", string(buf[0:n]), " from ", raddr)

		if err != nil {
			fmt.Println("Error: ", err)
		}
		if buf[1] == 2 {
			sendAck(ServerConn, raddr, []byte(blocknum))
		} else { 
			sendAck(ServerConn, raddr, buf[2:4])
		}
		
	}
}
