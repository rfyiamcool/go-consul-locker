package consulocker

import (
	"encoding/json"
	"errors"
	"time"

	api "github.com/hashicorp/consul/api"
)

const (
	// DefaultLockRetryInterval how long we wait after a failed lock acquisition
	DefaultLockRetryInterval = 10 * time.Second

	// session ttl
	DefautSessionTTL = 30 * time.Second

	TryAcquireMode = iota
	CallEventModel
)

var (
	defaultLogger = func(tmpl string, s ...interface{}) {}

	ErrKeyNameNull = errors.New("key is null")
)

// null logger
type loggerType func(tmpl string, s ...interface{})

func SetLogger(logger loggerType) {
	defaultLogger = logger
}

// DisLocker configured for lock acquisition
type DisLocker struct {
	doneChan          chan struct{}
	IsLocked          bool
	ConsulClient      *api.Client
	Key               string
	SessionID         string
	LockRetryInterval time.Duration
	SessionTTL        time.Duration
}

// Config is used to configure creation of client
type Config struct {
	Address           string
	KeyName           string        // key on which lock to acquire
	LockRetryInterval time.Duration // interval at which attempt is done to acquire lock
	SessionTTL        time.Duration // time after which consul session will expire and release the lock
}

func (c *Config) check() error {
	if c.KeyName == "" {
		return ErrKeyNameNull
	}

	return nil
}

func NewConfig() *Config {
	c := &Config{
		LockRetryInterval: DefaultLockRetryInterval,
		SessionTTL:        DefautSessionTTL,
	}
	return c
}

// New returns a new dislocker object
func New(o *Config) (*DisLocker, error) {
	var (
		locker DisLocker
	)

	// set consul server address
	cfg := api.DefaultConfig()
	cfg.Address = o.Address
	if o.Address == "" {
		cfg.Address = "127.0.0.1"
	}

	// instance consul client
	consulClient, err := api.NewClient(cfg)
	if err != nil {
		defaultLogger("new consul clinet failed, err: %v", err)
		return &locker, err
	}

	// set
	locker.doneChan = make(chan struct{})
	locker.ConsulClient = consulClient
	locker.Key = o.KeyName
	locker.LockRetryInterval = o.LockRetryInterval
	locker.SessionTTL = o.SessionTTL

	if err = o.check(); err != nil {
		return &locker, err
	}

	return &locker, nil
}

// RetryLockAcquire attempts to acquire the lock at `LockRetryInterval`
func (d *DisLocker) RetryLockAcquire(value map[string]string, acquired chan<- bool, released chan<- bool, errorChan chan<- error) {
	ticker := time.NewTicker(d.LockRetryInterval)

	for ; true; <-ticker.C {
		value["lock_time"] = time.Now().Format(time.RFC3339)
		lock, err := d.acquireLock(value, CallEventModel, released)
		if err != nil {
			defaultLogger("error on acquireLock :", err, "retry in -", d.LockRetryInterval)
			errorChan <- err
			continue
		}

		if lock {
			defaultLogger("lock acquired with consul session - %s", d.SessionID)
			ticker.Stop()
			acquired <- true
			break
		}
	}
}

// TryLockAcquire
func (d *DisLocker) TryLockAcquire(value map[string]string) (bool, error) {
	locked, err := d.acquireLock(value, TryAcquireMode, nil)
	if err != nil {
		defaultLogger("acquireLock failed, err: %v", err)
		return locked, err
	}

	if !locked {
		defaultLogger("can't acquire lock, session: %s", d.SessionID)
		return locked, nil
	}

	d.IsLocked = locked
	return locked, nil
}

func (d *DisLocker) ReleaseLock() error {
	if d.SessionID == "" {
		defaultLogger("cannot destroy empty session")
		return nil
	}

	_, err := d.ConsulClient.Session().Destroy(d.SessionID, nil)
	if err != nil {
		return err
	}

	defaultLogger("destroyed consul session: %s", d.SessionID)
	d.IsLocked = false
	return nil
}

// Renew incr key ttl
func (d *DisLocker) Renew() {
	d.ConsulClient.Session().Renew(d.SessionID, nil)
}

func (d *DisLocker) StartRenewProcess() {
	go func() {
		d.ConsulClient.Session().RenewPeriodic(d.SessionTTL.String(), d.SessionID, nil, d.doneChan)
	}()
}

func (d *DisLocker) createSession() (string, error) {
	return createSession(d.ConsulClient, d.Key, d.SessionTTL)
}

func (d *DisLocker) recreateSession() error {
	sessionID, err := d.createSession()
	if err != nil {
		return err
	}

	d.SessionID = sessionID
	return nil
}

func (d *DisLocker) acquireLock(value map[string]string, mode int, released chan<- bool) (bool, error) {
	if d.SessionID == "" {
		err := d.recreateSession()
		if err != nil {
			return false, err
		}
	}

	b, err := json.Marshal(value)
	if err != nil {
		defaultLogger("error on value marshal", err)
	}

	lock, err := d.ConsulClient.LockOpts(&api.LockOptions{Key: d.Key, Value: b, Session: d.SessionID, LockWaitTime: 1 * time.Second, LockTryOnce: true})
	if err != nil {
		return false, err
	}

	a, _, err := d.ConsulClient.Session().Info(d.SessionID, nil)
	if err == nil && a == nil {
		defaultLogger("consul session: %s is invalid now", d.SessionID)
		d.SessionID = ""
		return false, nil
	}

	if err != nil {
		return false, err
	}

	resp, err := lock.Lock(nil)
	if err != nil {
		return false, err
	}

	if resp == nil {
		return false, nil
	}

	if mode == CallEventModel {
		go func() {
			// wait event
			<-resp
			// close renew process
			close(d.doneChan)
			defaultLogger("lock released with session: %s", d.SessionID)
			d.IsLocked = false
			released <- true
		}()
	}

	return true, nil
}

func createSession(client *api.Client, consulKey string, ttl time.Duration) (string, error) {
	agentChecks, err := client.Agent().Checks()
	if err != nil {
		defaultLogger("error on getting checks, err: %v", err)
		return "", err
	}

	checks := []string{}
	checks = append(checks, "serfHealth")
	for _, j := range agentChecks {
		checks = append(checks, j.CheckID)
	}

	sessionID, _, err := client.Session().Create(&api.SessionEntry{Name: consulKey, Checks: checks, LockDelay: 0 * time.Second, TTL: ttl.String()}, nil)
	if err != nil {
		return "", err
	}

	defaultLogger("created consul session: %s", sessionID)
	return sessionID, nil
}
