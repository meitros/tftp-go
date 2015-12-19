package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
)

var debugflag bool

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

//
// packet parsing
//

// finds a null byte in n, or -1 if not present
func findNull(n []byte) int {
	for i := 0; i < len(n); i++ {
		if n[i] == 0 {
			return i
		}
	}
	return -1
}

// the fields dict will print byte values by default. We usually want to print
// strings (for debugging purposes)
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
