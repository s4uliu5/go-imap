package main

import (
	"bufio"
	"crypto/tls"
	"os"
	"log"
	"strings"
	"fmt"
	"net/textproto"
	"io"
	"strconv"
	"sync"
)

func check(err os.Error) {
	if err != nil {
		panic(err)
	}
}

type Status int
const (
	OK = Status(iota)
	NO
	BAD
)
func (s Status) String() string {
	return []string{
		"OK",
		"NO",
		"BAD",
	}[s];
}

type Tag int
const Untagged = Tag(-1)

type Response struct {
	status Status
	text string
}

type IMAP struct {
	// Client thread.
	nextTag int

	// Background thread.
	r *textproto.Reader
	w io.Writer

	lock sync.Mutex
	pending map[Tag]chan *Response
}

func NewIMAP() *IMAP {
	return &IMAP{pending:make(map[Tag]chan *Response)}
}

func (imap *IMAP) Connect(hostport string) (string, os.Error) {
	log.Printf("dial")
	conn, err := tls.Dial("tcp", hostport, nil)
	if err != nil {
		return "", err
	}

	imap.r = textproto.NewReader(bufio.NewReader(conn))
	imap.w = conn

	log.Printf("readline")
	tag, text, err := imap.ReadLine()
	if err != nil {
		return "", err
	}
	if tag != Untagged {
		return "", fmt.Errorf("expected untagged server hello. got %q", text)
	}

	status, text, err := ParseStatus(text)
	if status != OK {
		return "", fmt.Errorf("server hello %v %q", status, text)
	}

	imap.StartLoops()

	return text, nil
}

func splitToken(text string) (string, string) {
	space := strings.Index(text, " ")
	if space < 0 {
		return text, ""
	}
	return text[:space], text[space+1:]
}

func (imap *IMAP) ReadLine() (Tag, string, os.Error) {
	line, err := imap.r.ReadLine()
	if err != nil {
		return Untagged, "", err
	}
	log.Printf("<-server %q", line)

	switch line[0] {
	case '*':
		return Untagged, line[2:], nil
	case 'a':
		tagstr, text := splitToken(line)
		tagnum, err := strconv.Atoi(tagstr[1:])
		if err != nil {
			return Untagged, "", err
		}
		return Tag(tagnum), text, nil
	}

	return Untagged, "", fmt.Errorf("unexpected response %q", line)
}

func (imap *IMAP) Send(command string, ch chan *Response) (Tag, os.Error) {
	tag := Tag(imap.nextTag)
	toSend := []byte(fmt.Sprintf("a%d %s\r\n", int(tag), command))
	log.Printf("server<- %q...", toSend[0:10])

	if ch != nil {
		imap.lock.Lock()
		imap.pending[tag] = ch
		imap.lock.Unlock()
	}

	_, err := imap.w.Write(toSend)
	if err != nil {
		return Untagged, err
	}

	imap.nextTag++
	return tag, err
}

func (imap *IMAP) Auth(user string, pass string) (*Response, os.Error) {
	ch := make(chan *Response, 1)
	_, err := imap.Send(fmt.Sprintf("LOGIN %s %s", user, pass), ch)
	if err != nil {
		return nil, err
	}
	return <-ch, nil
}

func (imap *IMAP) StartLoops() {
	go func() {
		err := imap.ReadLoop()
		panic(err)
	}()
}

func (imap *IMAP) ReadLoop() os.Error {
	for {
		tag, text, err := imap.ReadLine()
		if err != nil {
			return err
		}
		text = text

		if tag == Untagged {
			resp, err := ParseResponse(text)
			if err != nil {
				return err
			}
			log.Printf("%v", resp)
		} else {
			status, text, err := ParseStatus(text)
			if err != nil {
				return err
			}

			imap.lock.Lock()
			ch := imap.pending[tag]
			imap.pending[tag] = nil, false
			imap.lock.Unlock()

			if ch != nil {
				log.Printf("wrote chan %v", status)
				ch <- &Response{status, text}
			}
		}
	}
	return nil
}

func ParseStatus(text string) (Status, string, os.Error) {
	// TODO: response code
	codes := map[string]Status{
		"OK": OK,
		"NO": NO,
		"BAD": BAD,
	}
	code, text := splitToken(text)

	status, known := codes[code]
	if !known {
		return BAD, "", fmt.Errorf("unexpected status %q", code)
	}
	return status, text, nil
}

type Capabilities struct {
	caps []string
}

func ParseResponse(text string) (interface{}, os.Error) {
	command, rest := splitToken(text)
	switch command {
	case "CAPABILITY":
		caps := strings.Split(rest, " ")
		return &Capabilities{caps}, nil
	}
	return nil, fmt.Errorf("unhandled untagged response %s", text)
}

func loadAuth(path string) (string, string) {
	f, err := os.Open(path)
	check(err)
	r := bufio.NewReader(f)

	user, isPrefix, err := r.ReadLine()
	check(err)
	if isPrefix {
		panic("prefix")
	}

	pass, isPrefix, err := r.ReadLine()
	check(err)
	if isPrefix {
		panic("prefix")
	}

	return string(user), string(pass)
}

func main() {
	user, pass := loadAuth("auth")

	imap := NewIMAP()
	text, err := imap.Connect("imap.gmail.com:993")
	check(err)
	log.Printf("connected %q", text)

	resp, err := imap.Auth(user, pass)
	log.Printf("%v", resp)
}
