package main

import (
	"errors"
	"log"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/shirou/gopsutil/disk"
	"golang.org/x/sync/errgroup"
)

type arguments struct {
	databasePath            string
	storageDeviceMountpoint string
}

func notify(title string, description string, timeout int) error {
	send, err := exec.LookPath("notify-send")
	if err != nil {
		return err
	}

	c := exec.Command(send, title, description, "-t", strconv.Itoa(timeout))
	if err := c.Run(); err != nil {
		return err
	}
	return nil
}

func notifyAndReplaceId(title string, description string, replaceId int, timeout int) error {
	send, err := exec.LookPath("notify-send")
	if err != nil {
		return err
	}

	c := exec.Command(send, title, description, "-r", strconv.Itoa(replaceId), "-t", strconv.Itoa(timeout))
	if err := c.Run(); err != nil {
		return err
	}
	return nil
}

func notifyAndGetId(title string, description string, timeout int) (int, error) {
	send, err := exec.LookPath("notify-send")
	if err != nil {
		return 0, err
	}
	notificationIdBytes, err := exec.Command(send, title, description, "-p", "-t", strconv.Itoa(timeout)).Output()
	if err != nil {
		return 0, err
	}
	notificationId, err := strconv.Atoi(strings.TrimSpace(string(notificationIdBytes)))
	return notificationId, nil
}

// Hack to display a notification until some condition is met
func notifyUntilClosure() func(title string, description string, condition func() bool) error {
	notificationId := -1

	return func(title string, description string, condition func() bool) error {
		// Do not harass if already true
		if condition() {
			return nil
		}
		const disappearingAnimationDur = 16
		const timeout = 50

		if notificationId == -1 {
			var err error
			notificationId, err = notifyAndGetId(title, description, timeout)
			if err != nil {
				return err
			}
		}

		for !condition() {
			err := notifyAndReplaceId(title, description, notificationId, timeout)
			if err != nil {
				return err
			}
			time.Sleep(time.Duration(timeout-disappearingAnimationDur) * time.Millisecond)
		}

		return nil
	}
}

func parse_args() (arguments, error) {
	var args arguments
	const arg_count = 2
	const help = `pbr is a small program that backs up and reminds you of backing up your passwords you silly goose
usage: pbr [database_path] [storage_device_mount_point]`
	if len(os.Args) < arg_count {
		return args, errors.New("Database path or storage device is not defined.\n" + help)
	} else if len(os.Args) > arg_count {
		return args, errors.New("Too many arguments.\n" + help)
	}

	args.databasePath = os.Args[0]
	args.storageDeviceMountpoint = os.Args[1]

	return args, nil
}

func wait_until_write(databasePath string) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	err = watcher.Add(databasePath)
	if err != nil {
		return err
	}

	eg := new(errgroup.Group)

	eg.Go(func() error {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return nil
				}

				/* By default when keepass saves, for example a password entry it first
				deletes the old db file and then moves the updated temp file into place.
				Checking for write in case direct write is configured. */
				if event.Has(fsnotify.Remove) || event.Has(fsnotify.Write) {
					log.Print("Write to database has been detected")
					return nil
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					return nil
				}

				return err
			}
		}
	})

	return eg.Wait()
}

func isMountpoint(mountpoint string) (bool, error) {
	partitions, err := disk.Partitions(false)
	if err != nil {
		return false, err
	}

	var mountpoints []string
	for _, partition := range partitions {
		mountpoints = append(mountpoints, partition.Mountpoint)
	}

	return slices.Contains(mountpoints, mountpoint), nil
}

func main() {
	args, err := parse_args()
	if err != nil {
		panic(err)
	}

	for {
		err := wait_until_write(args.databasePath)
		if err != nil {
			panic(err)
		}

		notify_until := notifyUntilClosure()
		err = notify_until("asdasd", "asdasd", func() bool {
			is_mountpoint, err := isMountpoint(args.storageDeviceMountpoint)
			if err != nil {
				panic(err)
			}

			if is_mountpoint {
				return true
			}
			return false
		})
		notify("Backing up the db", "", 0)
		db, ferr := os.Open(args.databasePath)
		if ferr != nil {
			return ferr
		}

	}

}
