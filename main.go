package main

import (
	"context"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
	"github.com/shirou/gopsutil/v4/process"
	"golang.org/x/sync/errgroup"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// https://kubernetes.io/zh-cn/docs/tasks/configure-pod-container/share-process-namespace/
// https://cctoctofx.netlify.app/post/cloud-computing/k8s-config-update/

func main() {
	time.Sleep(10 * time.Second)
	err := NewNginxReloadWatcher().Start()
	if err != nil {
		log.Println("[ERROR] start watcher err")
	}
}

type NginxReloadWatcher struct {
	sync.Mutex
	nginxMasterPid   int32
	timer            *time.Timer
	timeAfterSeconds time.Duration

	// file watch 的目录，用逗号分割
	// 如 /etc/nginx,/etc/nginx/conf.d
	watchDirs []string
}

func NewNginxReloadWatcher() *NginxReloadWatcher {

	timeAfterSecondsEnv := os.Getenv("TIME_AFTER_SECONDS")
	var timeAfter = 30
	atoi, err := strconv.Atoi(timeAfterSecondsEnv)
	if err == nil {
		timeAfter = atoi
	}
	log.Println("TIME_AFTER_SECONDS: ", timeAfter)

	watchDirsEnv := os.Getenv("WATCH_DIRS")

	log.Println("WATCH_DIRS: ", watchDirsEnv)
	watchDirs := strings.Split(watchDirsEnv, ",")
	log.Println("WATCH_DIRS after split: ", watchDirs)

	return &NginxReloadWatcher{
		timeAfterSeconds: time.Duration(timeAfter) * time.Second,
		watchDirs:        watchDirs,
	}
}

func (n *NginxReloadWatcher) findNginxMaterPid() (int32, error) {
	processes, err := process.Processes()
	if err != nil {
		return 0, err
	}

	for _, p := range processes {
		name, err := p.Name()
		if err != nil {
			return 0, errors.Wrap(err, "process Name err")
		}
		if name != "nginx" {
			continue
		}
		ppid, err := p.Ppid()
		if err != nil {
			return 0, errors.Wrap(err, "process ppid err")
		}
		if ppid == 0 {
			return p.Pid, nil
		}
	}
	return 0, errors.New("not found nginx master")
}

func (n *NginxReloadWatcher) Start() error {
	pid, err := n.findNginxMaterPid()
	if err != nil {
		return err
	}
	log.Println(fmt.Sprintf("nginx master id %d", pid))
	n.nginxMasterPid = pid

	g, gctx := errgroup.WithContext(context.TODO())
	g.Go(func() error {
		killSignal := make(chan os.Signal, 1)
		signal.Notify(killSignal, os.Interrupt, syscall.SIGTERM, os.Kill)
		<-killSignal
		return errors.New("kill signal")
	})

	g.Go(func() error {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return err
		}
		defer watcher.Close()

		// Add a path.
		for _, parentDir := range n.watchDirs {
			parentDir = fmt.Sprintf("/proc/%d/root%s", pid, parentDir)
			err = watcher.Add(parentDir)
			if err != nil {
				return err
			}
			err = filepath.WalkDir(parentDir, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					fmt.Printf("无法访问路径 %q: %v\n", path, err)
					return err // 继续遍历其他文件/目录
				}
				if d.IsDir() {
					watcher.Add(path)
				}
				return nil
			})
			if err != nil {
				return errors.Wrap(err, "遍历目录时出错")
			}
		}

		watchList := watcher.WatchList()

		for _, item := range watchList {
			log.Println(item)
		}

		for {
			select {
			case <-gctx.Done():
				return errors.New("gctx done")
			case event, ok := <-watcher.Events:
				if !ok {
					continue
				}
				log.Println("event:", event)
				if event.Has(fsnotify.Create) || event.Has(fsnotify.Write) || event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename) || event.Has(fsnotify.Chmod) {
					n.timerHubSignal(event.String())
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					continue
				}
				log.Println("error:", err)
			}
		}

	})
	return g.Wait()
}

func (n *NginxReloadWatcher) timerHubSignal(event string) {

	n.Lock()
	defer n.Unlock()

	if n.timer != nil {
		log.Println("[INFO] stop preview timer")
		n.timer.Stop()
	}
	//
	log.Println("[INFO] set new timer ", event)
	n.timer = time.AfterFunc(n.timeAfterSeconds, func() {
		n.Lock()
		defer n.Unlock()
		n.timer = nil

		command := exec.Command("/bin/bash", "-c", fmt.Sprintf("kill -HUP %d", n.nginxMasterPid))
		output, err := command.CombinedOutput()
		s := string(output)
		if err != nil {
			log.Println(fmt.Sprintf("kill -HUP %d err, err %v, output %s, event %s", n.nginxMasterPid, err, s, event))
			return
		}
		log.Println(fmt.Sprintf("kill -HUP %d success, output %s, event %s", n.nginxMasterPid, s, event))
	})
}
