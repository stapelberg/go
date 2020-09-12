// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build !cgo,linux osusergo,linux

package user

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net"
	"strconv"
)

// TODO: fallback to generic unix implementation
// (probably need another layer of indirection)

var userdbdAddress = "/run/systemd/userdb/io.systemd.NameServiceSwitch"

func queryUserdbd(method string, unmarshal func([]byte) (bool, error)) error {
	// systemd v245’s systemd-userdbd.service(8) (enabled by default) listens on
	// this Unix socket:
	// TODO: should we prefer io.systemd.Multiplexer?
	conn, err := net.Dial("unix", userdbdAddress) //io.systemd.Multiplexer")
	if err != nil {
		return err
	}
	defer conn.Close()

	// The other end of this socket is implemented in
	// https://github.com/systemd/systemd/tree/v245/src/userdb

	type params struct {
		Service string `json:"service"`
	}
	req := struct {
		Method     string `json:"method"`
		Parameters params `json:"parameters"`
		More       bool   `json:"more"`
	}{
		Method: method,
		Parameters: params{
			Service: "io.systemd.NameServiceSwitch",
		},
		More: true,
	}

	b, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if _, err := conn.Write(append(b, 0)); err != nil {
		return err
	}
	sc := bufio.NewScanner(conn)
	// This is bufio.ScanLines, but looking for a 0-byte instead of '\n':
	sc.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}
		if i := bytes.IndexByte(data, 0); i >= 0 {
			// We have a full 0-byte-terminated line.
			return i + 1, data[0:i], nil
		}
		// If we're at EOF, we have a final, non-terminated line. Return it.
		if atEOF {
			return len(data), data, nil
		}
		// Request more data.
		return 0, nil, nil
	})
	for sc.Scan() {
		continues, err := unmarshal(sc.Bytes())
		if err != nil {
			return nil
		}
		if !continues {
			break
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}

	return nil
}

type groupRecord struct {
	GroupName string `json:"groupName"`
	Gid       int64  `json:"gid"`
}

func lookupGroup(groupname string) (*Group, error) {
	return queryGroupDb(func(g *groupRecord) bool {
		return g.GroupName == groupname
	})
}

func lookupGroupId(id string) (*Group, error) {
	return queryGroupDb(func(g *groupRecord) bool {
		return strconv.FormatInt(g.Gid, 10) == id
	})
}

func queryGroupDb(predicate func(*groupRecord) bool) (*Group, error) {
	const method = "io.systemd.UserDatabase.GetGroupRecord"
	var group *Group
	unmarshal := func(b []byte) (bool, error) {
		var reply struct {
			Parameters struct {
				Record groupRecord `json:"record"`
			} `json:"parameters"`
			Continues bool `json:"continues"`
		}
		if err := json.Unmarshal(b, &reply); err != nil {
			return false, err
		}
		r := reply.Parameters.Record // for convenience
		if !predicate(&r) {
			return reply.Continues, nil // skip
		}
		group = &Group{
			Name: r.GroupName,
			Gid:  strconv.FormatInt(r.Gid, 10),
		}
		return reply.Continues, nil
	}
	if err := queryUserdbd(method, unmarshal); err != nil {
		return nil, err
	}
	return group, nil
}

type userRecord struct {
	UserName      string `json:"userName"`
	RealName      string `json:"realName"`
	Uid           int64  `json:"uid"`
	Gid           int64  `json:"gid"`
	HomeDirectory string `json:"homeDirectory"`
}

func lookupUser(username string) (*User, error) {
	return queryUserDb(func(u *userRecord) bool {
		return u.UserName == username
	})
}

func lookupUserId(uid string) (*User, error) {
	return queryUserDb(func(u *userRecord) bool {
		return strconv.FormatInt(u.Uid, 10) == uid
	})
}

func queryUserDb(predicate func(*userRecord) bool) (*User, error) {
	const method = "io.systemd.UserDatabase.GetUserRecord"
	var u *User
	unmarshal := func(b []byte) (bool, error) {
		var reply struct {
			Parameters struct {
				Record userRecord `json:"record"`
			} `json:"parameters"`
			Continues bool `json:"continues"`
		}
		if err := json.Unmarshal(b, &reply); err != nil {
			return false, err
		}
		r := reply.Parameters.Record // for convenience
		if !predicate(&r) {
			return reply.Continues, nil // skip
		}
		u = &User{
			Uid:      strconv.FormatInt(r.Uid, 10),
			Gid:      strconv.FormatInt(r.Gid, 10),
			Username: r.UserName,
			Name:     r.RealName,
			HomeDir:  r.HomeDirectory,
		}
		return reply.Continues, nil
	}
	if err := queryUserdbd(method, unmarshal); err != nil {
		return nil, err
	}
	return u, nil
}
