package rtsp

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"github.com/deepch/rtsp/sdp"
	"html"
	"io"
	"log"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type RtspClient struct {
	Debug         bool
	rtsptimeout   time.Duration
	rtptimeout    time.Duration
	keepalivetime int
	cseq          int
	uri           string
	host          string
	port          string
	login         string
	password      string
	session       string
	bauth         string
	nonce         string
	realm         string
	sdp           string
	track         []string
	socket        net.Conn
	firstvideots  int
	firstaudiots  int
	Signals       chan bool
	Outgoing      chan []byte
}

func RtspClientNew() *RtspClient {
	return &RtspClient{cseq: 1, rtsptimeout: 3, rtptimeout: 10, keepalivetime: 20, Signals: make(chan bool, 1), Outgoing: make(chan []byte, 100000)}
}
func (this *RtspClient) Open(uri string) (err error) {
	if err := this.ParseUrl(uri); err != nil {
		return err
	}
	if err := this.Connect(); err != nil {
		return err
	}
	if err := this.Write("OPTIONS", "", "", false, false); err != nil {
		return err
	}
	if err := this.Write("DESCRIBE", "", "", false, false); err != nil {
		return err
	}
	i := 0
	p := 1
	for _, track := range this.track {
		if err := this.Write("SETUP", "/"+track, "Transport: RTP/AVP/TCP;unicast;interleaved="+strconv.Itoa(i)+"-"+strconv.Itoa(p)+"\r\n", false, false); err != nil {
			return err
		}
		i++
		p++
	}
	if err := this.Write("PLAY", "", "", false, false); err != nil {
		return err
	}
	go this.RtspRtpLoop()
	return
}
func (this *RtspClient) Connect() (err error) {
	option := &net.Dialer{Timeout: this.rtsptimeout * time.Second}
	if socket, err := option.Dial("tcp", this.host+":"+this.port); err != nil {
		return err
	} else {
		this.socket = socket
	}
	return
}

func (this *RtspClient) Write(method string, track, add string, stage bool, noread bool) (err error) {
	this.cseq += 1
	status := 0
	if err := this.socket.SetDeadline(time.Now().Add(this.rtsptimeout * time.Second)); err != nil {
		return err
	}
	message := method + " " + this.uri + track + " RTSP/1.0\r\nCSeq: " + strconv.Itoa(this.cseq) + "\r\n" + add + this.session + this.Dauth(method) + this.bauth + "User-Agent: Lavf57.8.102\r\n\r\n"
	if this.Debug {
		log.Println(message)
	}
	if _, err := this.socket.Write([]byte(message)); err != nil {
		return err
	}
	if noread {
		return
	}
	if responce, err := this.Read(); err != nil {
		return err
	} else {
		fline := strings.SplitN(string(responce), " ", 3)
		if status, err = strconv.Atoi(fline[1]); err != nil {
			return err
		}
		if status == 401 && !stage {
			this.bauth = "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(this.login+":"+this.password)) + "\r\n"
			this.nonce = ParseDirective(string(responce), "nonce")
			this.realm = ParseDirective(string(responce), "realm")
			if err := this.Write(method, "", "", true, false); err != nil {
				return err
			}
		} else if status == 401 {
			return errors.New("Method " + method + " Authorization failed")
		} else if status != 200 {
			return errors.New("Method " + method + " Return bad status code")
		} else {
			switch method {
			case "SETUP":
				this.ParseSetup(string(responce))
			case "DESCRIBE":
				this.ParseDescribe(string(responce))
			case "PLAY":
				this.ParsePlay(string(responce))
			}
		}
	}
	return
}
func (this *RtspClient) Read() (buffer []byte, err error) {
	buffer = make([]byte, 4096)
	if err = this.socket.SetDeadline(time.Now().Add(this.rtsptimeout * time.Second)); err != nil {
		return nil, err
	}
	if n, err := this.socket.Read(buffer); err != nil || n <= 2 {
		return nil, err
	} else {
		if this.Debug {
			log.Println(string(buffer[:n]))
		}
		return buffer[:n], nil
	}
}
func (this *RtspClient) ParseUrl(uri string) (err error) {
	elemets, err := url.Parse(html.UnescapeString(uri))
	if err != nil {
		return err
	}
	if host, port, err := net.SplitHostPort(elemets.Host); err == nil {
		this.host = host
		this.port = port
	} else {
		this.host = elemets.Host
		this.port = "554"
	}
	if elemets.User != nil {
		this.login = elemets.User.Username()
		this.password, _ = elemets.User.Password()
	}
	if elemets.RawQuery != "" {
		this.uri = "rtsp://" + this.host + ":" + this.port + elemets.Path + "?" + elemets.RawQuery
	} else {
		this.uri = "rtsp://" + this.host + ":" + this.port + elemets.Path
	}
	return
}
func (this *RtspClient) Dauth(phase string) string {
	dauth := ""
	if this.nonce != "" {
		hs1 := this.GetMD5Hash(this.login + ":" + this.realm + ":" + this.password)
		hs2 := this.GetMD5Hash(phase + ":" + this.uri)
		responce := this.GetMD5Hash(hs1 + ":" + this.nonce + ":" + hs2)
		dauth = `Authorization: Digest username="` + this.login + `", realm="` + this.realm + `", nonce="` + this.nonce + `", uri="` + this.uri + `", response="` + responce + `"` + "\r\n"
	}
	return dauth
}
func ParseDirective(message, name string) string {
	index := strings.Index(message, name)
	if index == -1 {
		return ""
	}
	start := 1 + index + strings.Index(message[index:], `"`)
	end := start + strings.Index(message[start:], `"`)
	return strings.TrimSpace(message[start:end])
}
func (this *RtspClient) GetMD5Hash(text string) string {
	hash := md5.Sum([]byte(text))
	return hex.EncodeToString(hash[:])
}
func (this *RtspClient) ParseSetup(message string) {
	mparsed := strings.Split(message, "\r\n")
	for _, element := range mparsed {
		if strings.Contains(element, "Session:") {
			if strings.Contains(element, ";") {
				fist := strings.Split(element, ";")[0]
				this.session = "Session: " + fist[9:] + "\r\n"
			} else {
				this.session = "Session: " + element[9:] + "\r\n"
			}
		}
	}
}
func (this *RtspClient) ParseDescribe(message string) {
	sdpstring := strings.Split(message, "\r\n\r\n")
	if len(sdpstring) > 1 {
		this.sdp = sdpstring[1]
		for _, info := range sdp.Decode(sdpstring[1]) {
			this.track = append(this.track, info.Control)
		}
	} else {
		if this.Debug {
			log.Println("SDP not found")
		}
	}
}
func (this *RtspClient) ParsePlay(message string) {
	fist := true
	mparsed := strings.Split(message, "\r\n")
	for _, element := range mparsed {
		if strings.Contains(element, "RTP-Info") {
			mparseds := strings.Split(element, ",")
			for _, elements := range mparseds {
				mparsedss := strings.Split(elements, ";")
				if len(mparsedss) > 2 {
					if fist {
						this.firstvideots, _ = strconv.Atoi(mparsedss[2][8:])
						fist = false
					} else {
						this.firstaudiots, _ = strconv.Atoi(mparsedss[2][8:])
					}
				}
			}
		}
	}
}
func (this *RtspClient) RtspRtpLoop() {
	defer func() {
		this.Signals <- true
	}()
	header := make([]byte, 4)
	payload := make([]byte, 16384)
	sync_b := make([]byte, 1)
	timer := time.Now()
	start_t := true

	for {
		if int(time.Now().Sub(timer).Seconds()) > this.keepalivetime {
			if err := this.Write("OPTIONS", "", "", false, true); err != nil {
				return
			}
			timer = time.Now()
		}
		if start_t {
			this.socket.SetDeadline(time.Now().Add(50 * time.Second))
		} else {
			this.socket.SetDeadline(time.Now().Add(this.rtptimeout * time.Second))
		}
		if n, err := io.ReadFull(this.socket, header); err != nil || n != 4 {
			if this.Debug {
				log.Println("read header error", err)
			}
			return
		}
		if header[0] != 36 {
			rtsp := false
			if string(header) != "RTSP" {
				if this.Debug {
					log.Println("desync strange data repair", string(header), header, this.uri)
				}
			} else {
				rtsp = true
			}
			i := 1
			for {
				i++
				if i > 4096 {
					if this.Debug {
						log.Println("desync fatal miss position rtp packet", this.uri)
					}
					return
				}
				if n, err := io.ReadFull(this.socket, sync_b); err != nil && n != 1 {
					return
				}
				if sync_b[0] == 36 {
					header[0] = sync_b[0]
					if n, err := io.ReadFull(this.socket, sync_b); err != nil && n != 1 {
						return
					}
					if sync_b[0] == 0 || sync_b[0] == 1 || sync_b[0] == 2 || sync_b[0] == 3 {
						header[1] = sync_b[0]
						if n, err := io.ReadFull(this.socket, header[2:]); err != nil && n == 2 {
							return
						}
						if !rtsp {
							if this.Debug {
								log.Println("desync fixed ok", sync_b[0], this.uri, i, "afrer byte")
							}
						}
						break
					} else {
						if this.Debug {
							log.Println("desync repair fail chanel incorect", sync_b[0], this.uri)
						}
					}
				}
			}
		}
		payloadLen := (int)(header[2])<<8 + (int)(header[3])
		if payloadLen > 16384 || payloadLen < 12 {
			if this.Debug {
				log.Println("fatal size desync", this.uri, payloadLen)
			}
			continue
		}
		if n, err := io.ReadFull(this.socket, payload[:payloadLen]); err != nil || n != payloadLen {
			if this.Debug {
				log.Println("read payload error", payloadLen, err)
			}
			return
		} else {
			start_t = false
			this.Outgoing <- append(header, payload[:n]...)
		}
	}
}
func (this *RtspClient) Close() {
	if this.socket != nil {
		if err := this.socket.Close(); err != nil {
		}
	}
}
