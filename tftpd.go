package main

import (
	"errors"
	"fmt"
	"net"
	"os"
)

func DieOnError(err error) {
	if err != nil {
		fmt.Println("Error: ", err)
		os.Exit(0)
	}
}

// finds a null byte in n, or -1 if not present
func findNull(n []byte) int {
	for i := 0; i < len(n); i++ {
		if n[i] == 0 {
			return i
		}
	}
	return -1
}

func parsePacket(packet []byte) (int, map[string][]byte, error) {
	// TODO: parse more robustly?
	switch opcode := int(packet[1]); opcode {
	case 1:
		// RRQ: Read Request
		fallthrough // parsing this is identical to WRQ
	case 2:
		// WRQ: Write Request
		opcodeName := map[int]string{1: "RRQ", 2: "WRQ"}[opcode]

		// grab the filename and mode strings
		firstNull := findNull(packet[2:])
		if firstNull == -1 {
			return opcode, nil, errors.New(opcodeName + ": unable to get filename")
		} else if firstNull == 0 {
			return opcode, nil, errors.New(opcodeName + ": filename is empty/missing")
		}
		firstNull += 2 // we started on byte 2
		filename := packet[2:firstNull]

		// TODO (robustness): check if firstNull was the last char
		secondNull := findNull(packet[firstNull+1:])
		if secondNull == -1 {
			return opcode, nil, errors.New(opcodeName + ": unable to get mode")
		} else if secondNull == 0 {
			return opcode, nil, errors.New(opcodeName + ": mode is empty/missing")
		}
		secondNull += firstNull + 1 // again, add starting position
		mode := packet[firstNull+1 : secondNull]

		fields := map[string][]byte{
			"filename": filename,
			"mode":     mode,
		}
		return opcode, fields, nil
	case 3:
		// Data
		// TODO: robustness
		fields := map[string][]byte{
			"blocknum": packet[2:4],
			"data":     packet[4:],
		}
		return 3, fields, nil
	case 4:
		// Ack
		// TODO: robustness
		fields := map[string][]byte{
			"blocknum": packet[2:4],
		}
		return 4, fields, nil
	case 5:
		// Error
		nullPos := findNull(packet[4:])
		if nullPos == -1 {
			return 5, nil, errors.New("Error Packet: unable to get error message")
		}
		// an empty error message is ok
		errorMsg := []byte("")
		if nullPos > 0 {
			errorMsg = packet[4 : 4+nullPos]
		}

		fields := map[string][]byte{
			"code": packet[2:4],
			"msg":  errorMsg,
		}
		return 5, fields, nil
	default:
		return 0, nil, errors.New("Unknown opcode " + string(opcode))
	}
}

// the fields dict will print byte values by default. We usually want to print
// strings
func printableFields(fields map[string][]byte) map[string]string {
	printable := make(map[string]string)
	for key, value := range fields {
		printable[key] = string(value)
	}
	return printable
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
func sendAck(conn *net.UDPConn, addr *net.UDPAddr, blocknum []byte) {
	var ack = "\x00\x04"
	//	insertBlockNum(ack[2], blocknum)
	var res = append([]byte(ack), blocknum...)
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
		if n > 1024 {
			fmt.Println("packet is too large! we didn't have enough space, quitting :(")
			os.Exit(0)
		}
		if err != nil {
			fmt.Println("Error: ", err)
			continue
		}
		opcode, fields, err := parsePacket(buf[0:n])
		if err != nil {
			fmt.Println("Error: ", err)
			continue
		}
		fmt.Println("Recieved Packet, Opcode:", opcode, "Fields:", printableFields(fields))

		if opcode == 2 {
			sendAck(ServerConn, raddr, []byte(blocknum))
		} else {
			sendAck(ServerConn, raddr, fields["blocknum"])
		}
	}
}
