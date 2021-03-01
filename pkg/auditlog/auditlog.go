package auditlog

import (
	"encoding/gob"
	"net"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/common/log"
)

const (
	CreateOrUpdate Action = iota
	Delete
)

type Action int

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
	netLock sync.Mutex
	conn    *net.TCPConn
	timeout time.Duration
	dec     *gob.Decoder
	enc     *gob.Encoder
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

func NewAuditLogger(logger logr.Logger) (*AuditLoggerImpl, error) {
	auditLogger := new(AuditLoggerImpl)
	auditLogger.log = logger
	log.Info("Create new audit logger")
	err := auditLogger.Init()

	return auditLogger, err
}

func (a *AuditLoggerImpl) Init() error {
	connected := false
	retries := 0

	serverAddr, err := net.ResolveTCPAddr("tcp", ":10520")
	if err != nil {
		log.Error(err, "Failed to resolve TCP address")
		return err
	}

	for !connected && retries < 10 {
		a.conn, err = net.DialTCP("tcp", nil, serverAddr)
		if err != nil {
			log.Error(err, "Error connecting to Auditlog server ")
			retries++
			time.Sleep(time.Second * 5)
		} else {
			connected = true
		}
	}
	if err != nil {
		log.Error(err, "Failed to connect to TCP address")
		return err
	}

	a.enc = gob.NewEncoder(a.conn)
	a.dec = gob.NewDecoder(a.conn)
	a.timeout = time.Second * 5
	return nil
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
	var auditResponse AuditMessageResponse

	// synchronize multiple audit-requests by serializing messages
	a.netLock.Lock()
	defer a.netLock.Unlock()
	a.log.Info("Sending auditlog message")

	err := a.conn.SetWriteDeadline(time.Now().Add(a.timeout))
	a.checkError(err, "Error set deadline")

	err = a.enc.Encode(auditMessageInfo)
	if err != nil {
		a.log.Error(err, "Error while encoding audit message (will reconnect) ")
		_ = a.Init()
		return "", err
	}

	// wait for response:
	err = a.conn.SetReadDeadline(time.Now().Add(a.timeout))
	a.checkError(err, "Error set deadline")
	err = a.dec.Decode(&auditResponse)
	if err != nil {
		a.log.Error(err, "Decode error of audit message (will reconnect) ")
		_ = a.Init()
		return "", err
	}

	a.log.Info("Received answer from auditlog , id: " + auditResponse.ID)

	return auditResponse.ID, err
}
