package main

import (
	"errors"
	"io"
	"log"
	"os"
	"path"
	"slices"

	"pbr/notifications"

	"github.com/fsnotify/fsnotify"
	"github.com/shirou/gopsutil/disk"
	"golang.org/x/sync/errgroup"
)

type arguments struct {
	databasePath            string
	storageDeviceMountpoint string
}

type argumentError struct {
	s string
}

func (e *argumentError) Error() string {
	return e.s
}

func checkPath(path string) (bool, error) {
	_, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return true, nil
}

func parseArgs() (arguments, error) {
	var args arguments
	const arg_count = 3
	const help = `pbr is a small program that backs up and reminds you of backing up your passwords you silly goose
usage: pbr [database_path] [storage_device_mount_point]`
	if len(os.Args) < arg_count {
		return args, &argumentError{"Database path or storage device is not defined.\n" + help}
	} else if len(os.Args) > arg_count {
		return args, &argumentError{"Too many arguments.\n" + help}
	}

	for i := range os.Args {
		exists, err := checkPath(os.Args[i])
		if err != nil {
			return args, err
		}

		if !exists {
			return args, &argumentError{"Path does not exist: " + os.Args[i]}
		}
	}

	args.databasePath = os.Args[1]
	args.storageDeviceMountpoint = os.Args[2]

	return args, nil
}

func waitUntilWrite(databasePath string) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

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

func makeBackup(args arguments) error {
	db, err := os.Open(args.databasePath)
	if err != nil {
		return err
	}
	defer db.Close()

	var backedup_db *os.File
	backedup_db, err = os.Create(args.storageDeviceMountpoint + "/" + path.Base(args.databasePath))
	if err != nil {
		return err
	}
	defer backedup_db.Close()

	_, err = io.Copy(backedup_db, db)
	return err
}

func gracefulErrorOnExit(err error) {
	if arg_err, ok := err.(*argumentError); ok {
		log.Fatal(arg_err)
		os.Exit(1)
	}

	notifications.Notify("pbr encounterred an error", err.Error(), 0)
	panic(err)
}

func main() {
	args, err := parseArgs()
	if err != nil {
		gracefulErrorOnExit(err)
	}

	for {
		err := waitUntilWrite(args.databasePath)
		if err != nil {
			gracefulErrorOnExit(err)
		}

		is_mountpoint, err := isMountpoint(args.storageDeviceMountpoint)
		if err != nil {
			gracefulErrorOnExit(err)
		}

		if !is_mountpoint {
			notify_until := notifications.NotifyUntilClosure()
			err = notify_until("NOT A VALID MOUNTPOINT", "Please either change the mountpoint or plug in the backup drive", func() bool {
				is_mountpoint, err := isMountpoint(args.storageDeviceMountpoint)
				if err != nil {
					gracefulErrorOnExit(err)
				}

				if is_mountpoint {
					return true
				}
				return false
			})
		}

		notifications.Notify("Backing up the db", "", 0)
		err = makeBackup(args)
		if err != nil {
			gracefulErrorOnExit(err)
		}
	}
}
