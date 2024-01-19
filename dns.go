package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
	"github.com/nettica-com/nettica-admin/model"
	log "github.com/sirupsen/logrus"
)

var (
	ServerTable  map[string]string
	ServerLock   sync.Mutex
	DnsTable     map[string][]string
	DnsLock      sync.Mutex
	Resolvers    []string
	ResolverLock sync.Mutex
	DnsServers   map[string]*dns.Server
)

func StartDNS() error {
	ServerTable = make(map[string]string)
	DnsTable = make(map[string][]string)
	DnsServers = make(map[string]*dns.Server)

	dns.HandleFunc(".", handleQueries)

	InitializeDNS()

	var conf []byte
	for exists := false; !exists; {
		file, err := os.Open(GetDataPath() + "nettica.json")
		if err != nil {
			time.Sleep(time.Second)
		} else {
			conf, err = io.ReadAll(file)
			file.Close()
			if err != nil {
				log.Errorf("Error reading nettica config file: %v", err)
				time.Sleep(time.Second)
			} else {
				exists = true
			}
		}
	}

	var msg model.Message
	err := json.Unmarshal(conf, &msg)
	if err != nil {
		log.Errorf("Error reading message from config file")
		return err
	}

	ServerLock.Lock()
	defer ServerLock.Unlock()

	DnsLock.Lock()
	defer DnsLock.Unlock()

	ResolverLock.Lock()
	defer ResolverLock.Unlock()

	// get the DNS servers from the config file

	for i := 0; i < len(msg.Config); i++ {
		index := -1
		for j := 0; j < len(msg.Config[i].VPNs); j++ {
			if msg.Config[i].VPNs[j].DeviceID == device.Id {
				index = j
				break
			}
		}
		if index == -1 {
			log.Errorf("Error reading message %v", msg)
		} else {
			if msg.Config[i].VPNs[index].Enable && msg.Config[i].VPNs[index].Current.EnableDns {
				host := msg.Config[i].VPNs[index]
				name := strings.ToLower(host.Name)
				log.Infof("label = %s addr = %v", name, host.Current.Address)
				DnsTable[name] = append(DnsTable[name], host.Current.Address...)
				if strings.Contains(host.Current.Address[0], ":") {
					// ipv6
				} else {
					// ipv4
					addresses := strings.Split(host.Current.Address[0], "/")
					address := addresses[0]
					digits := strings.Split(address, ".")
					label := fmt.Sprintf("%s.%s.%s.%s.in-addr.arpa", digits[3], digits[2], digits[1], digits[0])
					DnsTable[label] = []string{name}
					log.Infof("label = %s name = %s", label, name)
				}
				msg.Config[i].VPNs = append(msg.Config[i].VPNs[:index], msg.Config[i].VPNs[index+1:]...)
				for j := 0; j < len(msg.Config[i].VPNs); j++ {
					n := strings.ToLower(msg.Config[i].VPNs[j].Name)
					if strings.Contains(msg.Config[i].VPNs[j].Current.Address[0], ":") {
						// ipv6

					} else {
						// ipv4
						addresses := strings.Split(msg.Config[i].VPNs[j].Current.Address[0], "/")
						address := addresses[0]
						digits := strings.Split(address, ".")
						label := fmt.Sprintf("%s.%s.%s.%s.in-addr.arpa", digits[3], digits[2], digits[1], digits[0])
						DnsTable[label] = []string{n}
					}
					log.Infof("label = %s name = %v", n, msg.Config[i].VPNs[j].Current.Address)
					DnsTable[n] = append(DnsTable[n], msg.Config[i].VPNs[j].Current.Address...)
					if msg.Config[i].VPNs[j].Current.Endpoint != "" {
						ip_port := msg.Config[i].VPNs[j].Current.Endpoint
						parts := strings.Split(ip_port, ":")
						ip := parts[0]
						ServerTable[ip] = ip
					}
				}
				resolvers := host.Current.Dns
				// remove the host address from the list of resolvers
				for j := 0; j < len(resolvers); j++ {
					parts := strings.Split(host.Current.Address[0], "/")
					if len(parts) > 0 && resolvers[j] == parts[0] {
						resolvers = append(resolvers[:j], resolvers[j+1:]...)
						break
					}
				}

				Resolvers = append(Resolvers, resolvers...)

				// Eliminate any duplicates in the Resolvers list
				for i := 0; i < len(Resolvers); i++ {
					for j := i + 1; j < len(Resolvers); j++ {
						if Resolvers[i] == Resolvers[j] {
							Resolvers = append(Resolvers[:j], Resolvers[j+1:]...)
							j--
						}
					}
				}

				if len(host.Current.Address[0]) > 3 {
					address := host.Current.Address[0][:len(host.Current.Address[0])-3] + ":53"
					server, err := LaunchDNS(address)
					if err != nil {
						log.Errorf("Error starting DNS server: %v", err)
					} else {
						DnsServers[address] = server
					}
				}
			}
		}
	}

	log.Infof("DNS Resolvers: %v", Resolvers)

	return nil
}

func StopDNS(address string) error {

	log.Infof("******************** STOP DNS : %s ********************", address)

	address += ":53"
	server := DnsServers[address]
	DnsServers[address] = nil

	if server != nil {
		server.Shutdown()
	}

	return nil
}

func UpdateDNS(msg model.Message) error {

	serverTable := make(map[string]string)
	dnsTable := make(map[string][]string)
	resolvers := make([]string, 0)

	for i := 0; i < len(msg.Config); i++ {
		index := -1
		for j := 0; j < len(msg.Config[i].VPNs); j++ {
			if msg.Config[i].VPNs[j].DeviceID == device.Id {
				index = j
				break
			}
		}
		if index == -1 {
			log.Errorf("Error reading message for DNS update: %v", msg)
			return errors.New("Error reading message")
		} else {
			if msg.Config[i].VPNs[index].Enable && msg.Config[i].VPNs[index].Current.EnableDns {
				host := msg.Config[i].VPNs[index]
				name := strings.ToLower(host.Name)
				dnsTable[name] = append(dnsTable[name], host.Current.Address...)
				if strings.Contains(host.Current.Address[0], ":") {
					// ipv6
				} else {
					// ipv4
					addresses := strings.Split(host.Current.Address[0], "/")
					address := addresses[0]
					digits := strings.Split(address, ".")
					label := fmt.Sprintf("%s.%s.%s.%s.in-addr.arpa", digits[3], digits[2], digits[1], digits[0])
					dnsTable[label] = []string{name}
					log.Infof("label = %s name = %s", label, name)

				}
				dnsTable[name] = append(dnsTable[name], host.Current.Address...)
				msg.Config[i].VPNs = append(msg.Config[i].VPNs[:index], msg.Config[i].VPNs[index+1:]...)
				for j := 0; j < len(msg.Config[i].VPNs); j++ {
					n := strings.ToLower(msg.Config[i].VPNs[j].Name)
					if strings.Contains(msg.Config[i].VPNs[j].Current.Address[0], ":") {
						// ipv6
					} else {
						// ipv4
						addresses := strings.Split(msg.Config[i].VPNs[j].Current.Address[0], "/")
						address := addresses[0]
						digits := strings.Split(address, ".")
						label := fmt.Sprintf("%s.%s.%s.%s.in-addr.arpa", digits[3], digits[2], digits[1], digits[0])
						dnsTable[label] = []string{n}
					}
					dnsTable[n] = append(dnsTable[n], msg.Config[i].VPNs[j].Current.Address...)
					if msg.Config[i].VPNs[j].Current.Endpoint != "" {
						ip_port := msg.Config[i].VPNs[j].Current.Endpoint
						parts := strings.Split(ip_port, ":")
						ip := parts[0]
						serverTable[ip] = ip
					}
				}
				resolver := host.Current.Dns
				// remove the host address from the list of resolvers
				for j := 0; j < len(resolver); j++ {
					parts := strings.Split(host.Current.Address[0], "/")
					if len(parts) > 0 && resolver[j] == parts[0] {
						resolver = append(resolver[:j], resolver[j+1:]...)
						break
					}
				}

				resolvers = append(resolvers, resolver...)

				// Eliminate any duplicates in the resolvers list
				for i := 0; i < len(resolvers); i++ {
					for j := i + 1; j < len(resolvers); j++ {
						if resolvers[i] == resolvers[j] {
							resolvers = append(resolvers[:j], resolvers[j+1:]...)
							j--
						}
					}
				}

				address := host.Current.Address[0]
				address = address[0:strings.Index(address, "/")] + ":53"

				server, _ := LaunchDNS(address)
				DnsServers[address] = server
			}
		}
	}
	DnsLock.Lock()
	DnsTable = dnsTable
	DnsLock.Unlock()

	ServerLock.Lock()
	ServerTable = serverTable
	ServerLock.Unlock()

	ResolverLock.Lock()
	log.Infof("Update DNS Resolvers: %v", resolvers)
	Resolvers = resolvers
	ResolverLock.Unlock()

	return nil
}

func handleQueries(w dns.ResponseWriter, r *dns.Msg) {
	var rr dns.RR

	q := strings.ToLower(r.Question[0].Name)
	q = strings.Trim(q, ".")

	if !device.Quiet {
		log.Infof("DNS Query: %s", q)
	}

	switch r.Question[0].Qtype {

	case dns.TypeA, dns.TypeAAAA:

		addrs := DnsTable[q]
		if addrs != nil {
			log.Infof("--- Query from DnsTable: %s", q)
			m := new(dns.Msg)
			m.SetReply(r)
			m.Compress = true

			if r.Question[0].Qtype == dns.TypePTR {
				rr = &dns.PTR{Hdr: dns.RR_Header{Name: r.Question[0].Name,
					Rrtype: dns.TypePTR,
					Class:  dns.ClassINET,
					Ttl:    300},
					Ptr: addrs[0] + ".",
				}
				m.Answer = append(m.Answer, rr)
				m.Authoritative = true
				m.Rcode = dns.RcodeSuccess
			}

			if r.Question[0].Qtype == dns.TypeA {
				offset := rand.Intn(len(addrs))
				for i := 0; i < len(addrs); i++ {
					x := (offset + i) % len(addrs)
					if !strings.Contains(addrs[x], ":") {
						ip, _, _ := net.ParseCIDR(addrs[x])
						rr = &dns.A{Hdr: dns.RR_Header{Name: r.Question[0].Name,
							Rrtype: dns.TypeA,
							Class:  dns.ClassINET,
							Ttl:    300},
							A: ip.To4(),
						}
						m.Answer = append(m.Answer, rr)
						m.Authoritative = true
						m.Rcode = dns.RcodeSuccess
					}
				}
			}
			if r.Question[0].Qtype == dns.TypeAAAA {
				offset := rand.Intn(len(addrs))
				for i := 0; i < len(addrs); i++ {
					x := (offset + i) % len(addrs)
					if strings.Contains(addrs[x], ":") {
						ip, _, _ := net.ParseCIDR(addrs[x])
						rr = &dns.AAAA{Hdr: dns.RR_Header{Name: r.Question[0].Name,
							Rrtype: dns.TypeAAAA,
							Class:  dns.ClassINET,
							Ttl:    300},
							AAAA: ip.To16(),
						}
						m.Answer = append(m.Answer, rr)
						m.Authoritative = true
						m.Rcode = dns.RcodeSuccess
					}
				}
			}
			w.WriteMsg(m)
			go LogMessage(q)
			return
		} else {
			QueryDNS(w, r)
			return
		}
	}

	QueryDNS(w, r)

}

// Make a recursive query
func QueryDNS(w dns.ResponseWriter, r *dns.Msg) {
	go LogMessage(r.Question[0].Name)
	c := new(dns.Client)
	c.Net = "udp"
	c.Timeout = 5000 * time.Millisecond

	// Measure the time it takes to get a response
	start := time.Now()

	for i := 0; i < len(Resolvers); i++ {
		log.Infof("*** Resolver: %s Query: %s", Resolvers[i], r.Question[0].Name)
		r, _, err := c.Exchange(r, Resolvers[i]+":53")
		end := time.Now()
		if err == nil {
			took := end.Sub(start)
			log.Infof("*** Response: (%v) %v", took, r)
			w.WriteMsg(r)
			return
		} else {
			log.Errorf("*** Error:   %v", err)
		}
	}
}

// This sends a multicast message with the DNS query to anyone listening
func LogMessage(query string) {

	NotifyDNS(query)

	/*
		raddr, err := net.ResolveUDPAddr("udp", "224.1.1.1:25264")
		if err != nil {
			return
		}

		conn, err := net.DialUDP("udp", nil, raddr)
		if err != nil {
			return
		}
		defer conn.Close()

		conn.WriteMsgUDP([]byte(query), nil, raddr)

		fmt.Fprint(conn, query)
	*/
}
