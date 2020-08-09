package controllers

import (
	"fmt"
	"github.com/coreos/bbolt"
	"github.com/cwarner818/giota"
	"github.com/iota-tangle-io/dtlg/backend/utilities"
	"github.com/iota-tangle-io/iota-spamalot.go"
	"gopkg.in/inconshreveable/log15.v2"
	"math/rand"
	"runtime"
	"strings"
	"sync"
	"time"
)

const DefaultMessage = "DISTRIBUTED9TANGLE9LOAD9GENERATOR"
const DefaultTag = "999DTLG"
const TagLength = 27
const AddressLength = 90

var TryteAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ9"

type StatusMsg struct {
	Running bool   `json:"running"`
	Node    string `json:"node"`
	PoW     string `json:"pow"`
	Tag     string `json:"tag"`
	Address string `json:"address"`
	Depth   int    `json:"depth"`
}

type SpammerCtrl struct {
	spammer *spamalot.Spammer
	metrics chan spamalot.Metric

	// synchronise access to spammer for now
	mu sync.Mutex

	muListeners    sync.Mutex
	listeners      map[int]chan interface{}
	nextListenerID int

	nodeURL  string
	powType  string
	tag      string
	address  string
	depth    int
	logger   log15.Logger
	database *spamalot.Database
}

func (ctrl *SpammerCtrl) createSpammer() *spamalot.Spammer {
	var address string
	for i := 0; i < 81; i++ {
		address += string(TryteAlphabet[rand.Intn(len(TryteAlphabet))])
	}

	var tag string
	if len(ctrl.tag) == 0 {
		tag = strings.ToUpper(DefaultTag + "9" + runtime.GOOS + "9" + ctrl.powType)
	} else {
		tag = ctrl.tag
	}
	ctrl.logger.Info(fmt.Sprintf("using tag %s", tag))

	if len(ctrl.address) != 0 {
		address = ctrl.address
	}
	ctrl.logger.Info(fmt.Sprintf("using address %s", address))

	if ctrl.database != nil {
		ctrl.database.Close()
	}

	var depth int
	if ctrl.depth != 0 {
		depth = ctrl.depth
	} else {
		depth = giota.Depth
	}
	ctrl.logger.Info(fmt.Sprintf("using depth %d", depth))

	db, err := bolt.Open("dtlg.db", 0600, nil)
	if err != nil {
		panic(err)
	}
	ctrl.database = spamalot.NewDatabase(db)

	powFunc := giota.GetAvailablePoWFuncs()[ctrl.powType]
	spammer, _ := spamalot.New(
		spamalot.WithMWM(int64(0)),
		spamalot.WithDepth(int64(depth)),
		spamalot.ToAddress(address),
		spamalot.WithTag(tag),
		spamalot.ToAddress(address),
		spamalot.WithMessage(DefaultMessage),
		spamalot.WithVerboseLogging(true),
		spamalot.WithSecurityLevel(spamalot.SecurityLevel(2)),
		spamalot.WithMetricsRelay(ctrl.metrics),
		spamalot.WithStrategy(""),
		spamalot.WithPoW(powFunc),
		spamalot.WithMessageMetrics(true),
		spamalot.WithDatabase(ctrl.database),
		spamalot.WithNode(ctrl.nodeURL, false),
	)

	return spammer
}

func (ctrl *SpammerCtrl) Init() error {
	ctrl.nodeURL = ""
	ctrl.metrics = make(chan spamalot.Metric)
	ctrl.listeners = map[int]chan interface{}{}
	l, err := utilities.GetLogger("spammer")
	if err != nil {
		return err
	}
	ctrl.logger = l
	ctrl.depth = giota.Depth

	powType, _ := giota.GetBestPoW()
	availablePoWs := giota.GetAvailablePoWFuncs()
	var a string
	var i int
	for powTypeName := range availablePoWs {
		i++
		if i == len(availablePoWs) {
			a += powTypeName
			continue
		}
		a += fmt.Sprintf("%s, ", powTypeName)
	}

	// auto. switch to PoW C on Mac
	_, hasPoWC := availablePoWs["PowC"]
	if runtime.GOOS == "darwin" && hasPoWC {
		powType = "PowC"
		ctrl.logger.Info(fmt.Sprintf("using PoW C because of Mac system detection out of [%s]", a))
	} else {
		ctrl.logger.Info(fmt.Sprintf("using preferred PoW '%s' out of [%s]", powType, a))
	}
	ctrl.powType = powType
	ctrl.spammer = ctrl.createSpammer()
	go ctrl.readMetrics()

	return nil
}

func (ctrl *SpammerCtrl) readMetrics() {
	for metric := range ctrl.metrics {
		ctrl.muListeners.Lock()
		for id, channel := range ctrl.listeners {
			select {
			case channel <- metric:
				// timeout
			case <-time.After(time.Duration(1) * time.Second):
				// auto remove if not writable within 1 second
				delete(ctrl.listeners, id)
				close(channel) // unwind listener
			}
		}
		ctrl.muListeners.Unlock()
	}
}

func (ctrl *SpammerCtrl) Start() error {
	ctrl.mu.Lock()
	defer ctrl.mu.Unlock()
	if ctrl.spammer.IsRunning() {
		return nil
	}
	go ctrl.spammer.Start()
	<-time.After(time.Duration(1) * time.Second)
	return nil
}

func (ctrl *SpammerCtrl) Stop() error {
	ctrl.mu.Lock()
	defer ctrl.mu.Unlock()
	if !ctrl.spammer.IsRunning() {
		return nil
	}
	ctrl.spammer.Stop()
	return nil
}

func (ctrl *SpammerCtrl) State() *StatusMsg {
	msg := &StatusMsg{}
	msg.Running = ctrl.spammer.IsRunning()
	msg.Node = ctrl.nodeURL
	msg.PoW = ctrl.powType
	msg.Tag = ctrl.tag
	msg.Address = ctrl.address
	msg.Depth = ctrl.depth
	return msg
}

func (ctrl *SpammerCtrl) ChangeNode(node string) {
	wasRunning := ctrl.spammer.IsRunning()
	ctrl.Stop()
	ctrl.nodeURL = node
	ctrl.spammer = ctrl.createSpammer()
	if wasRunning {
		go ctrl.spammer.Start()
		<-time.After(time.Duration(1) * time.Second)
	}
}

func (ctrl *SpammerCtrl) ChangeAddress(address string) {
	wasRunning := ctrl.spammer.IsRunning()
	ctrl.Stop()
	address = strings.TrimSpace(address)
	address = strings.ToUpper(address)
	for i := 0; i < len(address); i++ {
		letter := string(address[i])
		if strings.Index(TryteAlphabet, letter) == -1 {
			if i+1 == len(address) {
				address = address[:i] + "9"
			} else {
				address = address[:i] + "9" + address[i+1:]
			}
		}
	}

	toPad := AddressLength - len(address)
	if toPad > 0 {
		for ; toPad > 0; toPad-- {
			address += "9"
		}
	} else if toPad < 0 {
		address = address[:AddressLength]
	}

	ctrl.address = address
	ctrl.spammer = ctrl.createSpammer()
	if wasRunning {
		go ctrl.spammer.Start()
		<-time.After(time.Duration(1) * time.Second)
	}
}

func (ctrl *SpammerCtrl) ChangeDepth(depth int) {
	wasRunning := ctrl.spammer.IsRunning()
	ctrl.Stop()
	ctrl.depth = depth
	ctrl.spammer = ctrl.createSpammer()
	if wasRunning {
		go ctrl.spammer.Start()
		<-time.After(time.Duration(1) * time.Second)
	}
}

func (ctrl *SpammerCtrl) ChangeTag(tag string) {
	wasRunning := ctrl.spammer.IsRunning()
	ctrl.Stop()
	tag = strings.TrimSpace(tag)
	tag = strings.ToUpper(tag)
	for i := 0; i < len(tag); i++ {
		letter := string(tag[i])
		if strings.Index(TryteAlphabet, letter) == -1 {
			if i+1 == len(tag) {
				tag = tag[:i] + "9"
			} else {
				tag = tag[:i] + "9" + tag[i+1:]
			}
		}
	}
	toPad := TagLength - len(tag)
	if toPad > 0 {
		for ; toPad > 0; toPad-- {
			tag += "9"
		}
	} else if toPad < 0 {
		tag = tag[:TagLength]
	}
	ctrl.tag = tag
	ctrl.spammer = ctrl.createSpammer()
	if wasRunning {
		go ctrl.spammer.Start()
		<-time.After(time.Duration(1) * time.Second)
	}
}

func (ctrl *SpammerCtrl) ChangePoWType(newType string) {
	if _, ok := giota.GetAvailablePoWFuncs()[newType]; !ok {
		// crash because this should never be able to be reached in the first place
		panic("invalid PoW method supplied")
	}
	wasRunning := ctrl.spammer.IsRunning()
	ctrl.Stop()
	ctrl.logger.Info(fmt.Sprintf("changing PoW to %s", newType))
	ctrl.powType = newType
	ctrl.spammer = ctrl.createSpammer()
	if wasRunning {
		go ctrl.spammer.Start()
		<-time.After(time.Duration(1) * time.Second)
	}
}

func (ctrl *SpammerCtrl) AvailablePowTypes() []string {
	s := []string{}
	for n := range giota.GetAvailablePoWFuncs() {
		s = append(s, n)
	}
	return s
}

func (ctrl *SpammerCtrl) AddMetricListener(channel chan interface{}) int {
	ctrl.muListeners.Lock()
	defer ctrl.muListeners.Unlock()
	ctrl.nextListenerID++
	ctrl.listeners[ctrl.nextListenerID] = channel
	return ctrl.nextListenerID
}

func (ctrl *SpammerCtrl) RemoveMetricListener(id int) {
	ctrl.muListeners.Lock()
	defer ctrl.muListeners.Unlock()
	if _, ok := ctrl.listeners[id]; ok {
		delete(ctrl.listeners, id)
	}
}
