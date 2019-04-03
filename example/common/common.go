package common

import (
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/robfig/cron"
)

func MakeServerId() string {
	rand.Seed(time.Now().UnixNano())
	return fmt.Sprintf("%d", rand.Int63())
}

type MCron struct {
	cron *cron.Cron
}

func NewMCron() *MCron {
	mcron := &MCron{}
	c := cron.New()
	c.AddFunc("*/3 * * * * *", func() { fmt.Println("Every 3 sec") })
	mcron.cron = c
	return mcron
}

func (a *MCron) Start() {
	log.Println("cron started")
	a.cron.Start()
}

func (a *MCron) Stop() {
	log.Println("cron stopped")
	a.cron.Stop()
}
