package auditlog

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/go-logr/logr"
)

const (
	CreateOrUpdate Action = iota
	Delete
)

type Action int

var netLock sync.Mutex

func (a *Action) GetActionAsString() string {
	if a != nil {
		switch *a {
		case CreateOrUpdate:
			return "CreateOrUpdate"
		case Delete:
			return "Delete"
		default:
			return "Unknown"
		}
	}
	return "nil"
}

type AuditLogger interface {
	Log(auditMessageInfo *AuditMessageInfo) (string, error)
	Close() error
}

type AuditLoggerImpl struct {
	log     logr.Logger
	conn    *net.UDPConn
	buffer  bytes.Buffer
	timeout time.Duration
	recvbuf []byte
}

type AuditMessageInfo struct {
	Action      Action
	ClusterBOM  string
	ProjectName string
	ClusterName string
	ServiceUser string
	ClusterURL  string
	Bom         string
	OldBom      string
	ID          string // set on return
	Success     *bool
}

type AuditMessageResponse struct {
	ID string
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func NewAuditMessage(action Action, clusterBOM, projectName, clusterName, serviceUser, clusterURL, bom,
	oldBom string, success *bool) *AuditMessageInfo {
	msg := new(AuditMessageInfo)
	msg.Action = action
	msg.ClusterBOM = clusterBOM
	msg.ProjectName = projectName
	msg.ClusterName = clusterName
	msg.ServiceUser = serviceUser
	msg.ClusterURL = clusterURL
	msg.Bom = bom
	msg.OldBom = oldBom
	msg.ID = ""
	msg.Success = success
	return msg
}

func NewAuditLogger(log logr.Logger) (*AuditLoggerImpl, error) {
	ServerAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:10520")
	if err != nil {
		log.Error(err, "Failed to create UDP server")
		return nil, err
	}

	LocalAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		log.Error(err, "Failed to resolve UDP address")
		return nil, err
	}

	conn, err := net.DialUDP("udp", LocalAddr, ServerAddr)
	if err != nil {
		log.Error(err, "Failed to start UDP")
		return nil, err
	}

	auditLogger := new(AuditLoggerImpl)
	auditLogger.log = log
	auditLogger.conn = conn
	auditLogger.timeout = 5 * time.Second
	auditLogger.recvbuf = make([]byte, 65536)
	return auditLogger, nil
}

func (a *AuditLoggerImpl) Close() error {
	var err error = nil
	if a.conn != nil {
		err = a.conn.Close()
		if err != nil {
			a.log.Error(err, "Failed to close network connection for auditlog")
		}
	}
	return err
}

func (a *AuditLoggerImpl) checkError(err error, msg string) {
	if err != nil {
		a.log.Error(err, msg)
	}
}
func (a *AuditLoggerImpl) Log(auditMessageInfo *AuditMessageInfo) (string, error) {
	id := ""
	enc := gob.NewEncoder(&a.buffer)
	err := enc.Encode(auditMessageInfo)
	a.checkError(err, "Encode error of audit message")
	if err != nil {
		return id, err
	}

	// synchronize multiple audit-requests by serializing messages
	netLock.Lock()
	defer netLock.Unlock()
	a.log.Info("Sending auditlog message, len: " + strconv.Itoa(len(a.buffer.Bytes())))
	err = a.conn.SetWriteDeadline(time.Now().Add(a.timeout))
	a.checkError(err, "Error set deadline")

	// first send size of message:
	messageLength := len(a.buffer.Bytes())
	bufferForMessageLength := []byte(strconv.Itoa(messageLength))
	_, err = a.conn.Write(bufferForMessageLength)
	a.checkError(err, "Error sending audit message: ")
	const chunkSize = 4096
	nSent := chunkSize
	for offset := 0; offset < messageLength; offset += nSent {
		chunk := a.buffer.Bytes()[offset:min(messageLength, offset+chunkSize)]
		nSent, err = a.conn.Write(chunk)
		a.checkError(err, "Error sending audit message: ")
		if nSent < len(chunk) {
			fmt.Println("!!!WARNING: less sent than wanted: ", nSent)
		}
	}

	a.buffer.Reset()
	if err != nil {
		return id, err
	}

	// wait for response:
	err = a.conn.SetReadDeadline(time.Now().Add(a.timeout))
	a.checkError(err, "Error set deadline")
	n, _, err := a.conn.ReadFromUDP(a.recvbuf)
	if err != nil {
		a.log.Error(err, "Error while reading from UDP: ")
	} else if n > 0 {
		var auditResponse AuditMessageResponse
		dec := gob.NewDecoder(bytes.NewReader(a.recvbuf))
		err = dec.Decode(&auditResponse)
		if err != nil {
			a.log.Error(err, "Decode error of audit message")
		}
		a.buffer.Reset()
		a.log.Info("Received answer from auditlog (" + strconv.Itoa(n) + " bytes, id: " + id)
	}

	return id, err
}
