package main

import (
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/rfyiamcool/go-consul-locker"
	"github.com/rfyiamcool/go-consul-locker/example/common"
)

func main() {
	var (
		d       *consulocker.DisLocker
		err     error
		term    = make(chan os.Signal)
		running = true
		wg      = sync.WaitGroup{}
	)

	signal.Notify(term, os.Interrupt, syscall.SIGTERM)

	mcron := common.NewMCron()
	d, err = consulocker.New(
		&consulocker.Config{
			Address: "127.0.0.1:8500",
			KeyName: "lock/add_user",
		},
	)
	if err != nil {
		log.Println("Error ", err)
		return
	}

	value := map[string]string{
		"server_id": common.MakeServerId(),
	}

	wg.Add(1)
	go func() {
		defer wg.Done()

		select {
		case <-term:
			running = false
			if !d.IsLocked {
				return
			}

			if err := d.ReleaseLock(); err != nil {
				log.Println(err)
			}
			log.Println("release lock ok")

			mcron.Stop()
			log.Println("Exiting gracefully...")
			return
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			isLocked, err := d.TryLockAcquire(value)
			if !running {
				log.Println("running is false")
				return
			}

			if err != nil || isLocked == false {
				log.Printf("can't acquire lock, err: %v\n", err)
				time.Sleep(1 * time.Second)
				continue
			}

			mcron.Start()
			for running {
				d.Renew()
				time.Sleep(1 * time.Second)
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		time.Sleep(2 * time.Second)
		for running {
			log.Printf("isLocked state is %v", d.IsLocked)
			time.Sleep(2 * time.Second)
		}
	}()

	wg.Wait()
	log.Println("exit")
}
