package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rfyiamcool/go-consul-locker"
	"github.com/rfyiamcool/go-consul-locker/example/common"
)

func main() {
	var d *consulocker.DisLocker
	var err error

	go func() {
		mcron := common.NewMCron()
		d, err = consulocker.New(
			&consulocker.Config{
				Address:           "127.0.0.1:8500",
				KeyName:           "LockKV",
				LockRetryInterval: time.Second * 5,
			},
		)
		if err != nil {
			log.Println("Error ", err)
			return
		}

		acquireCh := make(chan bool)
		releaseCh := make(chan bool)
		errorCh := make(chan error)

		for {
			log.Println("try to acquire lock")
			value := map[string]string{
				"server_id": common.MakeServerId(),
			}

			go d.RetryLockAcquire(value, acquireCh, releaseCh, errorCh)
			select {
			case <-acquireCh:
				mcron.Start() // Start the cron when lock is acquired

			case err := <-errorCh:
				log.Println(err.Error())
				os.Exit(99)
			}

			<-releaseCh
			mcron.Stop() // Stop the cron when lock is released
		}
	}()

	// errCh := make(chan error)
	term := make(chan os.Signal)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)

	select {
	case <-term:
		if err := d.ReleaseLock(); err != nil {
			log.Println(err)
		}

		time.Sleep(1 * time.Second)
		log.Println("Exiting gracefully...")

		// case err := <-errCh:
		// 	log.Println("Error starting web server, exiting gracefully:", err)
		// }
	}
}
