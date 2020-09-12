// +build !cgo,linux osusergo,linux

package user

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func serveUserdbd(conn net.Conn) error {
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return err
	}
	buf = buf[:n]
	log.Printf("read line: %q (hex: %x)", buf, buf)

	if !bytes.HasSuffix(buf, []byte{0}) {
		return fmt.Errorf("message not \\0-terminated: %x", buf)
	}

	// TODO: inspect the query, verify it matches expectations:
	//
	// {\"method\":\"io.systemd.UserDatabase.GetGroupRecord\",\"parameters\":{\"service\":\"io.systemd.NameServiceSwitch\"},\"more\":true}\x00

	if _, err := conn.Write(append([]byte(`{"parameters":{"record":{"groupName":"stdlibcontrib","gid":181,"status":{"ecb5a44f1a5846ad871566e113bf8937":{"service":"io.systemd.NameServiceSwitch"}}},"incomplete":false}}`), 0)); err != nil {
		return err
	}

	return nil
}

func TestQueryUserdbd(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "testqueryuserdbd")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	sockFn := filepath.Join(tmpdir, "sock")

	// TODO: encapsulate parameters into a state struct and modify it there for testing
	userdbdAddress = sockFn

	ln, err := net.Listen("unix", sockFn)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				log.Print(err)
				return
			}
			go func() {
				if err := serveUserdbd(conn); err != nil {
					log.Print(err)
				}
			}()
		}
	}()

	g, err := lookupGroup("stdlibcontrib")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("g = %+v", g)
}
