package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/vishvananda/netns"
)

type NsMap map[int]int

func getNsMap() (NsMap, error) {
	procPaths, err := filepath.Glob("/proc/[0-9]*")
	if err != nil {
		log.Println(err)
		return nil, err
	}

	m := NsMap{}
	for _, procPath := range procPaths {
		_, pidStr := filepath.Split(procPath)
		identifierStr, err := os.Readlink(procPath + "/ns/net")
		if err != nil {
			log.Println(err)
			continue
		}
		identifierStr = strings.Split(identifierStr, ":")[1]
		identifierStr = identifierStr[1 : len(identifierStr)-1]

		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			log.Println(err)
			continue
		}
		identifier, err := strconv.Atoi(identifierStr)
		if err != nil {
			log.Println(err)
			continue
		}
		v, ok := m[identifier]
		if !ok || pid < v {
			m[identifier] = pid
		}
	}
	return m, nil
}

func getInterfaces(ctx context.Context, handle netns.NsHandle, identifier int) {
	// Lock the OS Thread so we don't accidentally switch namespaces
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	err := netns.Set(handle)
	if err != nil {
		log.Println(err)
		return
	}
	defer handle.Close()

	sleepTime := time.Second * 3
	timer := time.NewTimer(sleepTime)
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			ifaces, err := net.Interfaces()
			if err == nil {
				log.Printf("Interfaces at identifier %d: %v\n\n", identifier, ifaces)
			}
			timer = time.NewTimer(sleepTime)
		}
	}
}

func callAtExit(f func()) {
	go func() {
		c := make(chan os.Signal)
		signal.Notify(c, os.Interrupt)
		<-c
		log.Println("Caught interrupt")
		f()
	}()
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	callAtExit(cancel)
	m, err := getNsMap()
	if err != nil {
		log.Println(err)
		return
	}
	log.Printf("%d namespaces...\n", len(m))
	for netIdentifier, pid := range m {
		handle, err := netns.GetFromPid(pid)
		if err != nil {
			log.Printf("Can't get namespace handle from %d:%d\n,", netIdentifier, pid)
			continue
		}
		go getInterfaces(ctx, handle, netIdentifier)
	}
	<-ctx.Done()
	time.Sleep(time.Millisecond * 100)
}
