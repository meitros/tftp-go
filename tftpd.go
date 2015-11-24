package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"
)

type block struct {
	blocknum int
	nbytes   int
	buf      []byte
}

var debugflag bool
var chunk block
var ReadMode bool
var LastBlockSent bool //true if the lastBlock in the file has been transmitted

func DieOnError(err error) {
	if err != nil {
		fmt.Println("Error: ", err)
		os.Exit(0)
	}
}

func debugPrint(msg string) {
	if debugflag {
		fmt.Println(msg)
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
		ReadMode = true
		// RRQ: Read Request
		fallthrough // parsing this is identical to WRQ
	case 2:
		// WRQ: Write Request
		opcodeName := map[int]string{1: "RRQ", 2: "WRQ"}[opcode]

		if ReadMode && opcode != 1 {
			fmt.Println("The opcode received was unexpected")
		}
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

		// set the transferMode variable
		/*		if bytes.Equal(mode,[]byte("NetAscii")) {
					transferMode =NETASCII
				}
				transferMode = NETASCII
				if byies.Equal(mode,[]byte("Binary")) {
					transferMode = BINARY
				} */
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
		if key == "blocknum" || key == "code" {
			printable[key] = fmt.Sprintf("%d", binary.BigEndian.Uint16(value))
		} else {
			printable[key] = string(value)
		}
	}
	return printable
}

func sendAck(conn *net.UDPConn, addr *net.UDPAddr, blocknum []byte) {
	var ack = "\x00\x04"
	//	insertBlockNum(ack[2], blocknum)
	var res = append([]byte(ack), blocknum...)
	fmt.Println(res, "Sending ack code ", ack, "blocknum ", blocknum)
	_, err := conn.WriteToUDP(res, addr)
	if err != nil {
		fmt.Printf("Couldn't send response %v", err)
	}
}

func SendAChunk(conn *net.UDPConn, addr *net.UDPAddr) {
	var datablock []byte
	//Send the chink that we have in the Block (it is either the previous one being retransmitted or a the next one
	var opcode = "\x00\x03" //Data packet
	/*
		var nullbyte = "\x00"
		s16 := strconv.FormatUint(uint64(chunk.blocknum), 16)
		fmt.Println("=========== blocknum and conv",uint64(chunk.blocknum),"   ", s16)
		if chunk.blocknum < 16 {
			fmt.Println("Padding a zero byte,before zero",datablock[0:])
			datablock = append([]byte(opcode), []byte(nullbyte)...)
			fmt.Println("Padding a zero byte, after zero",datablock[0:])
		}
		sbytes := []byte(s16)
		fmt.Println(sbytes)
		fmt.Println("Adding the blocknum, datablock before num",datablock[0:])
		datablock = append([]byte(opcode), []byte(s16)...)
		fmt.Println("Adding the blocknum, datablock after num",datablock[0:])
		datablock = append(datablock,chunk.buf...)
	*/
	var blocknumBytes = make([]byte, 2)
	binary.BigEndian.PutUint16(blocknumBytes, uint16(chunk.blocknum))

	datablock = append(datablock, []byte(opcode)...)
	datablock = append(datablock, blocknumBytes...)
	fmt.Println("Stage 2: Datablock ", datablock)
	if chunk.nbytes < 512 {
		// make a slice of size chunk.nbytes and copy the data into it
		tempbuf := make([]byte, chunk.nbytes)
		n1 := copy(tempbuf, chunk.buf[0:chunk.nbytes])
		fmt.Println("Copied %d bytes to the last chunk being sent", n1)
		datablock = append(datablock, tempbuf...)
	} else {
		datablock = append(datablock, chunk.buf...)
	}
	fmt.Println("sending datablock", datablock)
	n2, err := conn.WriteToUDP(datablock, addr)
	fmt.Println("Number of Bytes sent is ", n2)
	if err != nil {
		fmt.Printf("Couldn't send datablock %v", err)
	}
}

/*
func LoadFile(filename string,transferMode string) *os.File{
    // open input file
    fi, err := os.Open(filename)
    if err != nil {
        panic(err)
    }
    // close fi on exit and check for its returned error
    defer func() {
        if err := fi.Close(); err != nil {
            panic(err)
        }
    }()
    // make a read buffer
	fmt.Println("Opened file ",filename)
//    r = bufio.NewReader(fi)
	chunk.blocknum = 0
	buf := make([]byte, 512)
	fmt.Println("Going to read the first block, file handle is ",fi)
	n, err := fi.Read(buf)
	chunk.nbytes = n
	chunk.buf = buf
	fmt.Println("chunk read", buf,"size ",n)
	return fi
}
*/
func ReadAChunk(inFile *os.File) int {
	// make a buffer to keep chunks that are read
	if LastBlockSent {
		return 0
	}
	LastBlockSent = false
	chunk.blocknum++
	if chunk.blocknum > 0 {
		mybuf := make([]byte, 512)
		// read a chunk
		fmt.Println("About to read Block num ", chunk.blocknum, "the handle is", inFile, "END OF HANDLE***************************")
		n, err := inFile.Read(mybuf)
		if err != nil && err != io.EOF {
			panic(err)
		}
		chunk.nbytes = n
		chunk.buf = mybuf
		if n < 512 {
			LastBlockSent = true
		}
		return n
	} else {
		return 512
	}
}

func main() {
	var inFile *os.File
	var err error
	port := os.Args[1]
	debugflag = false
	if strings.Compare(os.Args[2], "debug") == 0 {
		debugflag = true
	}
	ReadMode = false
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
		ServerConn.SetReadDeadline(time.Now().Add(5 * time.Second))
		n, raddr, err := ServerConn.ReadFromUDP(buf)
		if n > 1024 {
			fmt.Println("packet is too large! we didn't have enough space, quitting :(")
			os.Exit(0)
		}
		if err != nil {
			e, ok := err.(net.Error)
			if !ok || !e.Timeout() {
				// handle error, it's not a timeout
				fmt.Println("Error: ", err)
				continue
			}
			if e.Timeout() {
				//We timed out retransmit the block
				SendAChunk(ServerConn, raddr)
			}
		}
		opcode, fields, err := parsePacket(buf[0:n])
		if err != nil {
			fmt.Println("Error: ", err)
			continue
		}
		fmt.Println("Recieved Packet, Opcode:", opcode, "Fields:", printableFields(fields))

		if opcode == 1 {
			debugPrint("Processing opcode 1")
			//			inFile = LoadFile(string(fields["filename"]), string(fields["mode"]))
			inFile, err = os.Open(string(fields["filename"]))
			if err != nil {
				panic(err)
			}
			// close fi on exit and check for its returned error
			defer func() {
				if err := inFile.Close(); err != nil {
					panic(err)
				}
			}()
			debugPrint("File loaded, going to read chunk")
			n := ReadAChunk(inFile)
			fmt.Println("Read chunk with ", n, " bytes")
			SendAChunk(ServerConn, raddr)
		}
		if opcode == 2 {
			debugPrint("Processing opcode 2")
			sendAck(ServerConn, raddr, []byte(blocknum))
		}
		if opcode == 4 {
			//we are skipping check of block number for now, need to fix it for robustness
			//			if string(fields[blocknum]) == string(chunk.blocknum) {
			debugPrint("Processing opcode 4")
			fmt.Println()
			n := ReadAChunk(inFile)
			fmt.Println("Read chunk with ", n, " bytes")
			if n > 0 {
				SendAChunk(ServerConn, raddr)
			} else {
				fmt.Println("File is all transmitted")
				os.Exit(0)
			}
			//				}
		}
	}
}
