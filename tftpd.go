package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type block struct {
	blocknum int
	nbytes   int
	buf      []byte
}

// nts = note to self
// nts: only used in client code (RunClient / getNextCommand)
var ServerAddress string //address of the server to connect to
var binarymode bool      //true if the mode selected is binary. false = Default mode is Ascii
var fSource string       //name of the file to operate on
var fdest string         //namefunc

// nts: used by ReadAChunk / SendChunk
var chunk block
var LastBlockSent bool //true if the lastBlock in the file has been transmitted

// nts: ???
var ReadMode bool

//
// Sending packets (acks, data) and reading a from file [section]
//

func sendAck(isServer bool, conn *net.UDPConn, addr *net.UDPAddr, blocknum []byte) {
	var err error
	var ack = "\x00\x04"
	//	insertBlockNum(ack[2], blocknum)
	var res = append([]byte(ack), blocknum...)
	fmt.Println(res, "Sending ack code ", ack, "blocknum ", blocknum)
	if isServer {
		_, err = conn.WriteToUDP(res, addr)
	} else {
		_, err = conn.Write(res)
	}
	if err != nil {
		fmt.Printf("Couldn't send response %v", err)
	}
}

func SendAChunk(conn *net.UDPConn, addr *net.UDPAddr, isServer bool) {
	var n2 int
	var err error
	var datablock []byte
	//Send the chink that we have in the Block (it is either the previous one being retransmitted or a the next one
	var opcode = "\x00\x03" //Data packet
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
	if isServer {
		n2, err = conn.WriteToUDP(datablock, addr)
	} else {
		n2, err = conn.Write(datablock)
	}
	fmt.Println("Number of Bytes sent is ", n2)
	if err != nil {
		fmt.Printf("Couldn't send datablock %v", err)
	}
}

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

//
// Client/Server functions
//

func RunServer() {
	var port string

	fmt.Print("Enter the port to use:")
	fmt.Scanln(&port)

	var inFile *os.File
	var outFile *os.File

	ServerAddr, err := net.ResolveUDPAddr("udp", ":"+port)
	if err != nil {
		fmt.Println("We encountered an error when resolving address")
	}
	DieOnError(err)

	/* Now listen at selected port */
	ServerConn, err := net.ListenUDP("udp", ServerAddr)
	if err != nil {
		fmt.Println("We encountered an error when listening")
	}
	DieOnError(err)
	defer ServerConn.Close()

	fmt.Println("Port has been opened, listening")
	buf := make([]byte, 1024)

	//	nil blocknum
	var blocknum = "\x00\x00"
	ServerConn.SetReadDeadline(time.Now().Add(120 * time.Second))
	for {
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
				fmt.Println("We timed out, quitting")
				SendAChunk(ServerConn, raddr, true)
			}
		}
		opcode, fields, err := parsePacket(buf[0:n])
		if err != nil {
			fmt.Println("Error: ", err)
			continue
		}
		fmt.Println("Recieved Packet, Opcode:", opcode, "Fields:", printableFields(fields))

		if opcode == 1 {
			fmt.Println("Processing opcode 1, filename is ", string(fields["filename"]))
			//			inFile = LoadFile(string(fields["filename"]), string(fields["mode"]))
			inpFile, err := os.Open(string(fields["filename"]))
			if err != nil {
				panic(err)
			}
			inFile = inpFile
			// close fi on exit and check for its returned error
			defer func() {
				if err := inFile.Close(); err != nil {
					panic(err)
				}
			}()
			debugPrint("File loaded, going to read chunk")
			n := ReadAChunk(inFile)
			fmt.Println("Read chunk with ", n, " bytes")
			SendAChunk(ServerConn, raddr, true)
		}
		if opcode == 2 {
			debugPrint("Processing opcode 2")
			//open output file same name as the source file in local directory
			dir, file := filepath.Split(string(fields["filename"]))
			fmt.Println("dir and file are", dir, file)
			outFile, err = os.Create(file)
			fmt.Println("output file created", file)
			if err != nil {
				panic(err)
			}
			sendAck(true, ServerConn, raddr, []byte(blocknum))
		}
		if opcode == 3 {
			fmt.Println("Writing blocknum", []byte(fields["blocknum"]))
			n3, err := outFile.Write(fields["data"])
			fmt.Println("No, of bytes written", n3)
			if err != nil {
				panic(err)
			}
			sendAck(true, ServerConn, raddr, []byte(fields["blocknum"]))
			if n3 < 512 {
				//all done close the file
				outFile.Close()
				os.Exit(0)
			}
		}
		if opcode == 4 {
			//we are skipping check of block number for now, need to fix it for robustness
			//			if string(fields[blocknum]) == string(chunk.blocknum) {
			debugPrint("Processing opcode 4")
			fmt.Println()
			n := ReadAChunk(inFile)
			fmt.Println("Read chunk with ", n, " bytes")
			if n > 0 {
				SendAChunk(ServerConn, raddr, true)
			} else {
				fmt.Println("File is all transmitted")
				os.Exit(0)
			}
			//				}
		}
	}
}

func JoinStrings(s []string) string {
	//	s := []string{"foo", "bar", "baz"}
	sRet := strings.Join(s, "")
	return sRet
}

func RunClient() {
	var inFile *os.File
	var outFile *os.File

	ServerAddr, err := net.ResolveUDPAddr("udp", ServerAddress)
	DieOnError(err)
	fmt.Println("successfully resolved the server address")
	LocalAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	DieOnError(err)
	fmt.Println("successfully resolved the local address")

	ServerConn, err := net.DialUDP("udp", LocalAddr, ServerAddr)
	DieOnError(err)
	fmt.Println("successfully dialed the server")

	defer ServerConn.Close()

	//if ReadMode then Open or Create a file to save the received data
	if ReadMode {
		outFile, err = os.Create(fdest)
		if err != nil {
			panic(err)
		}
	} else {
		//open the infile to transmit
		inFile, err = os.Open(fSource)
		if err != nil {
			panic(err)
		}
	}
	zeroByte := "\x00"
	sOpCode := "\x00\x02" //opcode = 2 for PUT
	if ReadMode {
		sOpCode = "\x00\x01" //opcode=1 for GET
	}
	sMode := "NETASCII"
	if binarymode {
		sMode = "OCTET"
	}

	fmt.Sprintf("% x %s % x %s % x", sOpCode, fSource, zeroByte, sMode, zeroByte)
	s := []string{sOpCode, fSource, zeroByte, sMode, zeroByte}
	fmt.Println(s)

	msg := JoinStrings(s)
	fmt.Println("sending request to server ", []byte(msg))

	buf := []byte(msg)
	nchar, err := ServerConn.Write(buf)
	if nchar == 0 {
		fmt.Println("Error the number of characters sent is zero")
	}
	if err != nil {
		fmt.Println(msg, err)
	}
	for {
		buf := make([]byte, 1024)
		//Try to read from the connection for response from server
		n, raddr, err := ServerConn.ReadFromUDP(buf)
		fmt.Println("number of characters received ", n)
		fmt.Println("Return address is", raddr)
		if n > 1024 {
			fmt.Println("packet is too large! we didn't have enough space, quitting :(")
			//			os.Exit(0)
		}
		if err != nil {
			e, ok := err.(net.Error)
			if !ok || !e.Timeout() {
				// handle error, it's not a timeout
				fmt.Println("Error: ", err)
				continue
			}
			/*			if e.Timeout() {
						//We timed out retransmit the block
						SendAChunk(ServerConn, raddr)
					} */
		}
		opcode, fields, err := parsePacket(buf[0:n])
		if err != nil {
			fmt.Println("Error: ", err)
			continue
		}

		fmt.Println("Recieved Packet, Opcode:", opcode, "Fields:", printableFields(fields))
		// We are a client so we only expect an opcode 3(Data), opcode 4 (achknowledgement) or opcode 5 (error), opcode 1 and 2 are invalid
		if opcode == 1 || opcode == 2 {
			debugPrint("Processing opcode 1 or 2 ")
			/*			//			inFile = LoadFile(string(fields["filename"]), string(fields["mode"]))
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
			*/
		}
		if opcode == 3 {
			//received data block save it and acknowledge
			blocknum := fields["blocknum"]
			datablock := fields["data"]
			nout, err := outFile.Write(datablock)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			fmt.Println("About to send Ack to ServerConn and raddr", ServerConn, raddr)
			sendAck(false, ServerConn, raddr, blocknum)
			if nout < 512 {
				//close the outFile and end session
				outFile.Close()
				fmt.Println("Received the file ", fSource)
				return
			}
		}
		if opcode == 4 {
			//we are skipping check of block number for now, need to fix it for robustness
			//			if string(fields[blocknum]) == string(chunk.blocknum) {
			debugPrint("Processing opcode 4")
			fmt.Println()
			n := ReadAChunk(inFile)
			fmt.Println("Read chunk with ", n, " bytes")
			if n > 0 {
				SendAChunk(ServerConn, raddr, false)
			} else {
				fmt.Println("File is all transmitted")
				os.Exit(0)
			}
			//				}
		}
	}
}

func getNextCommand() int {
	//Prompt for next action
	sMode := ""
	sAction := ""

	for {
		fmt.Print("TFTP Client(-h for help):")
		n, err := fmt.Scanln(&ServerAddress, &sMode, &sAction, &fSource, &fdest)
		fmt.Println("Number of tokens", n)
		if err != nil {
			fmt.Println("Error occurred: ", err)
		}
		if strings.Compare(strings.ToLower(ServerAddress), "-h") == 0 {
			fmt.Println("Please enter Server Address transfermode(-i or -a) action(get or put) sourcefilename destinationfilename")
			fmt.Println("separate each value with a space in between")
			continue
		}
		if strings.Compare(strings.ToLower(ServerAddress), "quit") == 0 {
			return (-1)
		}
		if strings.Compare(strings.ToLower(ServerAddress), "stop") == 0 {
			return (-1)
		}
		if strings.Compare(sMode, "-i") == 0 {
			binarymode = true
		}
		if strings.Compare(strings.ToLower(sAction), "get") == 0 {
			ReadMode = true
		}
		return (0)
	}
}

func main() {
	n := len(os.Args)

	// The second argument will be client or server and the third argument is optional and can be debug
	isServer := false
	if strings.Compare(os.Args[1], "client") == 0 {
		// nothing to do
	} else if strings.Compare(os.Args[1], "server") == 0 {
		isServer = true
	} else {
		fmt.Println("Unknown command: Expecting either client or server")
		os.Exit(0)
	}

	debugflag = false
	if n > 2 {
		if strings.Compare(os.Args[1], "debug") == 0 {
			debugflag = true
		}
	}

	if isServer {
		RunServer()
	} else {
		for {
			// TODO: move shell code inside RunClient
			action := getNextCommand()
			if action == -1 {
				fmt.Println("Client finished session, Goodbye")
				os.Exit(0)
			}
			RunClient()
		}
	}
	fmt.Println("We are back from server or client finctions. Goodbye")
}
