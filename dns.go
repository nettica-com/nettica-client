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

type DNS struct {
	ServerTable   map[string]string
	DnsTable      map[string][]string
	Resolvers     []string
	DnsServers    map[string]*dns.Server
	SearchDomains []string
	Host          model.VPN
}

var (
	global     DNS
	globalLock sync.Mutex
)

var blackhole = []string{
	"168.192.in-addr.arpa",
	"10.in-addr.arpa",
	"16.172.in-addr.arpa",
	"17.172.in-addr.arpa",
	"18.172.in-addr.arpa",
	"19.172.in-addr.arpa",
	"20.172.in-addr.arpa",
	"21.172.in-addr.arpa",
	"22.172.in-addr.arpa",
	"23.172.in-addr.arpa",
	"24.172.in-addr.arpa",
	"25.172.in-addr.arpa",
	"26.172.in-addr.arpa",
	"27.172.in-addr.arpa",
	"28.172.in-addr.arpa",
	"29.172.in-addr.arpa",
	"30.172.in-addr.arpa",
	"31.172.in-addr.arpa",
	"254.169.in-addr.arpa",
	"2.0.192.in-addr.arpa",
	"100.51.198.in-addr.arpa",
	"113.0.203.in-addr.arpa",
	"0.8.e.f.ip6.arpa",
	"c.f.ip6.arpa",
	"d.f.ip6.arpa",
	"8.e.f.ip6.arpa",
	"9.e.f.ip6.arpa",
	"a.e.f.ip6.arpa",
	"b.e.f.ip6.arpa",
}

func StartDNS() error {
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

	d, err := ParseMessage(msg)
	if err != nil {
		log.Errorf("Error parsing message: %v", err)
		return err
	}

	globalLock.Lock()
	defer globalLock.Unlock()

	// loop through the dns server and stop them if they are not in the new list
	for address, server := range global.DnsServers {
		found := false
		for a := range d.DnsServers {
			if a == address {
				found = true
				d.DnsServers[a] = server
				break
			}
		}
		if !found {
			log.Infof("Stopping DNS Server: %s", address)
			server.Shutdown()
			delete(global.DnsServers, address)
		}
	}

	global = *d

	// loop through the dns servers and start them
	for address, server := range global.DnsServers {
		if server == nil {
			server, _ := LaunchDNS(address)
			if server != nil {
				global.DnsServers[address] = server
			}
		}
	}

	log.Infof("DNS Resolvers: %v", d.Resolvers)

	return nil
}

func ParseMessage(msg model.Message) (*DNS, error) {

	var d DNS

	d.ServerTable = make(map[string]string)
	d.DnsTable = make(map[string][]string)
	d.DnsServers = make(map[string]*dns.Server)
	d.SearchDomains = make([]string, 0)

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
			return nil, errors.New("Error reading message")
		} else {

			d.Host = msg.Config[i].VPNs[index]

			if msg.Config[i].VPNs[index].Enable && msg.Config[i].VPNs[index].Current.EnableDns {
				for j := 0; j < len(msg.Config[i].VPNs); j++ {
					name := strings.ToLower(msg.Config[i].VPNs[j].Name)
					addresses := strings.Split(msg.Config[i].VPNs[j].Current.Address[0], "/")
					address := addresses[0]
					if strings.Contains(address, ":") {
						label, err := formatIPv6PTR(address)
						if err != nil {
							log.Errorf("can't generate reverse DNS label for %s", address)
						} else {
							d.DnsTable[label] = []string{name}
						}
					} else {
						// ipv4
						digits := strings.Split(address, ".")
						label := fmt.Sprintf("%s.%s.%s.%s.in-addr.arpa", digits[3], digits[2], digits[1], digits[0])
						d.DnsTable[label] = []string{name}
					}
					d.DnsTable[name] = append(d.DnsTable[name], msg.Config[i].VPNs[j].Current.Address...)

					// add the server to the server table
					if msg.Config[i].VPNs[j].Current.Endpoint != "" {
						ip_port := msg.Config[i].VPNs[j].Current.Endpoint
						parts := strings.Split(ip_port, ":")
						if len(parts) > 0 {
							if len(parts[0]) == 2 {
								// this is an ipv4 address
								ip := parts[0]
								d.ServerTable[ip] = ip
							} else {
								// this is an ipv6 address
								parts = strings.Split(ip_port, "]")
								if len(parts) > 0 {
									ip := strings.Trim(parts[0], "[")
									d.ServerTable[ip] = ip
								}
							}
						}
					}
				}

				// remove the host address from the list of resolvers
				resolver := d.Host.Current.Dns
				for j := 0; j < len(resolver); j++ {
					parts := strings.Split(d.Host.Current.Address[0], "/")
					if len(parts) > 0 && resolver[j] == parts[0] {
						resolver = append(resolver[:j], resolver[j+1:]...)
						break
					}
				}

				d.Resolvers = append(d.Resolvers, resolver...)
				d.Resolvers = removeDuplicates(d.Resolvers)

				// Add the server address but no server
				if len(d.Host.Current.Address[0]) > 3 {
					address := d.Host.Current.Address[0][:len(d.Host.Current.Address[0])-3] + ":53"
					d.DnsServers[address] = nil
				}
			}

			// add the search domains.  the network name is a search domain
			d.SearchDomains = append(d.SearchDomains, strings.ToLower(d.Host.NetName))

			search := d.Host.Current.Dns
			for j := 0; j < len(search); j++ {
				ip := net.ParseIP(search[j])
				// if it's not an ip address then it's a search domain
				if ip == nil {
					d.SearchDomains = append(d.SearchDomains, search[j])
				}
			}
			d.SearchDomains = removeDuplicates(d.SearchDomains)

			// remove any non-ip address from the resolvers
			for j := 0; j < len(d.Resolvers); j++ {
				ip := net.ParseIP(d.Resolvers[j])
				if ip == nil {
					d.Resolvers = append(d.Resolvers[:j], d.Resolvers[j+1:]...)
					j--
				}
			}
		}
	}

	return &d, nil
}

func removeDuplicates(list []string) []string {
	for i := 0; i < len(list); i++ {
		for j := i + 1; j < len(list); j++ {
			if list[i] == list[j] {
				list = append(list[:j], list[j+1:]...)
				j--
			}
		}
	}
	return list
}

func formatIPv6PTR(address string) (string, error) {

	parts := strings.Split(address, ":")
	if len(parts) != 8 {
		log.Errorf("Can't generate reverse DNS label for %s", address)
	} else {
		for x := 0; x < 8; x++ {
			parts[x] = strings.Trim(parts[x], "[]")
			for len(parts[x]) < 4 {
				parts[x] = "0" + parts[x]
			}
		}
		label := fmt.Sprintf("%s.%s.%s.%s.%s.%s.%s.%s.%s.%s.%s.%s.%s.%s.%s.%s.%s.%s.%s.%s.%s.%s.%s.%s.%s.%s.%s.%s.%s.%s.%s.%s.ip6.arpa",
			parts[7][3:4], parts[7][2:3], parts[7][1:2], parts[7][0:1],
			parts[6][3:4], parts[6][2:3], parts[6][1:2], parts[6][0:1],
			parts[5][3:4], parts[5][2:3], parts[5][1:2], parts[5][0:1],
			parts[4][3:4], parts[4][2:3], parts[4][1:2], parts[4][0:1],
			parts[3][3:4], parts[3][2:3], parts[3][1:2], parts[3][0:1],
			parts[2][3:4], parts[2][2:3], parts[2][1:2], parts[2][0:1],
			parts[1][3:4], parts[1][2:3], parts[1][1:2], parts[1][0:1],
			parts[0][3:4], parts[0][2:3], parts[0][1:2], parts[0][0:1])

		return label, nil
	}
	return "", errors.New("can't generate reverse DNS label")
}

func StopDNS(address string) error {

	log.Infof("******************** STOP DNS : %s ********************", address)

	globalLock.Lock()
	defer globalLock.Unlock()

	if !strings.HasSuffix(address, ":53") {
		address += ":53"
	}
	server := global.DnsServers[address]

	// remove the server from the list of servers
	delete(global.DnsServers, address)

	if server != nil {
		server.Shutdown()
	}

	return nil
}

func DropCache() error {
	log.Infof("******************** DROP DNS CACHE ********************")

	globalLock.Lock()
	defer globalLock.Unlock()

	global.DnsTable = make(map[string][]string)

	return nil
}

func UpdateDNS(msg model.Message) error {
	log.Info("==================== UPDATE DNS ====================")
	result, err := ParseMessage(msg)
	if err != nil {
		return err
	}

	globalLock.Lock()
	defer globalLock.Unlock()

	// loop through the dns server and stop them if they are not in the new list
	for address, server := range global.DnsServers {
		found := false
		for a := range result.DnsServers {
			if a == address {
				found = true
				result.DnsServers[a] = server
				log.Infof("Keeping DNS Server: %s", server.Addr)
				break
			}
		}
		if !found {
			log.Infof("Stopping DNS Server: %s", address)
			server.Shutdown()
			delete(global.DnsServers, address)
		}
	}

	global = *result

	// loop through the dns servers and start any that need it
	for address, server := range global.DnsServers {
		if server == nil {
			log.Infof(" ************************************* Starting DNS Server: %s *************************************", address)
			server, err := LaunchDNS(address)
			if err != nil {
				log.Errorf("Error starting DNS Server: %v", err)
			}
			if server != nil {
				global.DnsServers[address] = server
			}
		}
	}

	log.Infof("DNS Resolvers: %v", global.Resolvers)

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

	case dns.TypeA, dns.TypeAAAA, dns.TypePTR:

		addrs := global.DnsTable[q]
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
func QueryDNS(w dns.ResponseWriter, query *dns.Msg) {

	defer LogMessage(query.Question[0].Name)

	q := strings.ToLower(query.Question[0].Name)
	q = strings.Trim(q, ".")

	fBLockSearch := false
	// Check for a search domain
	for i := 0; i < len(global.SearchDomains); i++ {
		if strings.HasSuffix(q, global.SearchDomains[i]) {

			fBLockSearch = true
			break
		}
	}

	fBlackhole := false

	// Check for and block blackhole domains
	for i := 0; i < len(blackhole); i++ {
		if strings.HasSuffix(q, blackhole[i]) {
			fBlackhole = true
			break
		}
	}

	// Query internal resolvers before external resolvers
	// x == 0 internal resolvers
	// x == 1 external resolvers

	for x := 0; x < 2; x++ {

		for i := 0; i < len(global.Resolvers); i++ {

			ip := net.ParseIP(global.Resolvers[i])
			if ip == nil {
				log.Errorf("Invalid IP address: %s", global.Resolvers[i])
				continue
			}

			if !ip.IsPrivate() && !ip.IsLoopback() && (fBlackhole || fBLockSearch) {

				log.Infof("Skipping %s", global.Resolvers[i])
				continue
			}

			// Manage when to call internal and external resolvers
			if x == 0 && !ip.IsPrivate() && !ip.IsLoopback() {
				continue
			}

			if x == 1 && (ip.IsPrivate() || ip.IsLoopback()) {
				continue
			}

			// Now make the query
			var err error
			response, err := MakeQuery(global.Resolvers[i]+":53", query)
			if err == nil {

				if response.Rcode == dns.RcodeSuccess {
					w.WriteMsg(response)
					return
				}

				log.Infof("--- Query to %s failed: %s %s", global.Resolvers[i], q, dns.RcodeToString[response.Rcode])

			} else {
				log.Errorf("*** Error:   %v", err)
			}
		}
	}

	if fBLockSearch {
		log.Infof("--- Query to SearchDomains blocked: %s", q)
		query.Authoritative = true
		query.Rcode = dns.RcodeNameError
		w.WriteMsg(query)
		return
	}
	if fBlackhole {
		log.Infof("--- Query to Blackhole blocked: %s", q)
		query.Authoritative = true
		query.Rcode = dns.RcodeNameError
		w.WriteMsg(query)
		return
	}

	query.Rcode = dns.RcodeServerFailure
	w.WriteMsg(query)
}

func MakeQuery(resolver string, q *dns.Msg) (*dns.Msg, error) {
	c := new(dns.Client)
	c.Net = "udp"
	c.Timeout = 1000 * time.Millisecond

	log.Infof("*** Resolver: %s Query: %s", resolver, q.Question[0].Name)

	// Measure the time it takes to get a response
	start := time.Now()

	r, _, err := c.Exchange(q, resolver)
	if err != nil {
		return nil, err
	}

	end := time.Now()
	took := end.Sub(start)

	if !device.Quiet {
		s := fmt.Sprintf("**** Response: %s (%v)   %s   %s   %s   ", resolver, took, dns.RcodeToString[r.Rcode], q.Question[0].Name, dns.Type(r.Question[0].Qtype).String())
		if r.Rcode == dns.RcodeSuccess && len(r.Answer) > 0 {
			s += fmt.Sprintf("%s   ", dns.Type(r.Answer[0].Header().Rrtype))
			if r.Answer[0].Header().Rrtype == dns.TypeA {
				s += r.Answer[0].(*dns.A).A.String()
			} else if r.Answer[0].Header().Rrtype == dns.TypeAAAA {
				s += r.Answer[0].(*dns.AAAA).AAAA.String()
			} else if r.Answer[0].Header().Rrtype == dns.TypePTR {
				s += r.Answer[0].(*dns.PTR).Ptr
			} else if r.Answer[0].Header().Rrtype == dns.TypeCNAME {
				s += r.Answer[0].(*dns.CNAME).Target
			}
		}
		log.Infof(s)
	}

	return r, nil
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
