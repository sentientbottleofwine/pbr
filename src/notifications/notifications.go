package notifications

import (
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func Notify(title string, description string, timeout int) error {
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

func NotifyAndReplaceId(title string, description string, replaceId int, timeout int) error {
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

func NotifyAndGetId(title string, description string, timeout int) (int, error) {
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
func NotifyUntilClosure() func(title string, description string, condition func() bool) error {
	notificationId := -1

	return func(title string, description string, condition func() bool) error {
		// Do not harass if already true
		if condition() {
			return nil
		}
		const frequency = 30

		if notificationId == -1 {
			var err error
			notificationId, err = NotifyAndGetId(title, description, 0)
			if err != nil {
				return err
			}
		}

		for true {
			err := NotifyAndReplaceId(title, description, notificationId, 0)
			if err != nil {
				return err
			}

			then := time.Now()
			for time.Since(then) < 30*time.Second {
				if condition() {
					return nil
				}
				time.Sleep(100 * time.Millisecond)
			}
		}

		return nil
	}
}
