package netpolicy

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

// BuiltinBackend implements a pure Go userspace TAP forwarder.
type BuiltinBackend struct {
	tap    *os.File
	iface  string
	cfg    *NetConfig
	pins   *PinnedIPSet
	mu     sync.Mutex
	closed bool
}

// NewBuiltinBackend opens /dev/net/tun and prepares the TAP interface.
func NewBuiltinBackend(cfg *NetConfig, pins *PinnedIPSet) (*BuiltinBackend, error) {
	f, err := os.OpenFile("/dev/net/tun", os.O_RDWR, 0)
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return nil, fmt.Errorf("builtin backend requires CAP_NET_ADMIN: %w", err)
		}
		return nil, fmt.Errorf("open /dev/net/tun: %w", err)
	}

	ifaceName := fmt.Sprintf("sbx%d", os.Getpid())
	ifr, err := unix.NewIfreq(ifaceName)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("NewIfreq: %w", err)
	}
	ifr.SetUint16(unix.IFF_TAP | unix.IFF_NO_PI)
	if err := unix.IoctlIfreq(int(f.Fd()), unix.TUNSETIFF, ifr); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("TUNSETIFF: %w", err)
	}

	return &BuiltinBackend{
		tap:   f,
		iface: ifaceName,
		cfg:   cfg,
		pins:  pins,
	}, nil
}

// Run reads L2 frames from the TAP device and handles allowed egress traffic.
func (b *BuiltinBackend) Run() error {
	buf := make([]byte, 65536)
	for {
		n, err := b.tap.Read(buf)
		if err != nil {
			b.mu.Lock()
			closed := b.closed
			b.mu.Unlock()
			if closed {
				return nil
			}
			return err
		}

		// Ethernet header (14 bytes) + IPv4 header (at least 20 bytes)
		if n < 14+20 {
			continue
		}

		ethType := binary.BigEndian.Uint16(buf[12:14])
		if ethType != 0x0800 { // IPv4 only
			continue
		}

		ipHdr := buf[14 : 14+20]
		ihl := int(ipHdr[0]&0x0F) * 4
		if ihl < 20 || n < 14+ihl {
			continue
		}

		proto := ipHdr[9]
		srcIP := net.IP(ipHdr[12:16])
		dstIP := net.IP(ipHdr[16:20])

		if !b.isIPAllowed(dstIP) {
			// Drop packet to non-allowed destination (deny-by-default)
			continue
		}

		switch proto {
		case 6: // TCP
			if n < 14+ihl+20 {
				continue
			}
			tcpHdr := buf[14+ihl : 14+ihl+20]
			srcPort := binary.BigEndian.Uint16(tcpHdr[0:2])
			dstPort := binary.BigEndian.Uint16(tcpHdr[2:4])
			payload := buf[14+ihl+20 : n]
			go b.relayTCP(srcIP, srcPort, dstIP, dstPort, payload, ipHdr, tcpHdr)
		case 17: // UDP
			if n < 14+ihl+8 {
				continue
			}
			udpHdr := buf[14+ihl : 14+ihl+8]
			srcPort := binary.BigEndian.Uint16(udpHdr[0:2])
			dstPort := binary.BigEndian.Uint16(udpHdr[2:4])
			payload := buf[14+ihl+8 : n]
			go b.relayUDP(srcIP, srcPort, dstIP, dstPort, payload, ipHdr, udpHdr)
		}
	}
}

func (b *BuiltinBackend) relayTCP(srcIP net.IP, srcPort uint16, dstIP net.IP, dstPort uint16, payload []byte, ipHdr, tcpHdr []byte) {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", dstIP.String(), dstPort), 5*time.Second)
	if err != nil {
		return
	}
	defer conn.Close()

	if len(payload) > 0 {
		if _, err := conn.Write(payload); err != nil {
			return
		}
	}

	resp := make([]byte, 1500)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		rn, rerr := conn.Read(resp)
		if rn > 0 {
			b.writeTCPResponse(dstIP, dstPort, srcIP, srcPort, resp[:rn])
		}
		if rerr != nil {
			break
		}
	}
}

func (b *BuiltinBackend) relayUDP(srcIP net.IP, srcPort uint16, dstIP net.IP, dstPort uint16, payload []byte, ipHdr, udpHdr []byte) {
	raddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", dstIP.String(), dstPort))
	if err != nil {
		return
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return
	}
	defer conn.Close()

	if len(payload) > 0 {
		if _, err := conn.Write(payload); err != nil {
			return
		}
	}

	resp := make([]byte, 1500)
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	rn, _, rerr := conn.ReadFrom(resp)
	if rn > 0 && rerr == nil {
		b.writeUDPResponse(dstIP, dstPort, srcIP, srcPort, resp[:rn])
	}
}

func (b *BuiltinBackend) writeTCPResponse(srcIP net.IP, srcPort uint16, dstIP net.IP, dstPort uint16, payload []byte) {
	tcpHdrLen := 20
	ipHdrLen := 20
	totalLen := 14 + ipHdrLen + tcpHdrLen + len(payload)
	frame := make([]byte, totalLen)

	// Ethernet header: dst=broadcast, src=locally administered MAC, ethType=0x0800
	frame[0], frame[1], frame[2], frame[3], frame[4], frame[5] = 0xff, 0xff, 0xff, 0xff, 0xff, 0xff
	frame[6] = 0x02
	binary.BigEndian.PutUint16(frame[12:14], 0x0800)

	// IPv4 header
	frame[14] = 0x45 // Version 4, IHL 5 (20 bytes)
	frame[15] = 0x00
	binary.BigEndian.PutUint16(frame[16:18], uint16(ipHdrLen+tcpHdrLen+len(payload)))
	frame[20], frame[21] = 0x00, 0x00 // ID
	frame[22] = 0x40                 // Flags: Don't Fragment
	frame[23] = 0x00                 // Fragment offset
	frame[24] = 64                   // TTL
	frame[25] = 6                    // TCP
	copy(frame[26:30], srcIP.To4())
	copy(frame[30:34], dstIP.To4())
	ipChk := checksum(frame[14:34])
	binary.BigEndian.PutUint16(frame[24:26], ipChk)

	// TCP header
	tcpOffset := 14 + ipHdrLen
	binary.BigEndian.PutUint16(frame[tcpOffset:tcpOffset+2], srcPort)
	binary.BigEndian.PutUint16(frame[tcpOffset+2:tcpOffset+4], dstPort)
	binary.BigEndian.PutUint32(frame[tcpOffset+4:tcpOffset+8], 1)  // Seq
	binary.BigEndian.PutUint32(frame[tcpOffset+8:tcpOffset+12], 1) // Ack
	frame[tcpOffset+12] = 0x50                                    // Data offset: 5 (20 bytes)
	frame[tcpOffset+13] = 0x18                                    // Flags: ACK | PSH
	binary.BigEndian.PutUint16(frame[tcpOffset+14:tcpOffset+16], 65535) // Window size
	copy(frame[tcpOffset+20:], payload)

	tcpChk := computeTCPChecksum(srcIP, dstIP, frame[tcpOffset:])
	binary.BigEndian.PutUint16(frame[tcpOffset+16:tcpOffset+18], tcpChk)

	b.mu.Lock()
	if !b.closed && b.tap != nil {
		_, _ = b.tap.Write(frame)
	}
	b.mu.Unlock()
}

func (b *BuiltinBackend) writeUDPResponse(srcIP net.IP, srcPort uint16, dstIP net.IP, dstPort uint16, payload []byte) {
	udpHdrLen := 8
	ipHdrLen := 20
	totalLen := 14 + ipHdrLen + udpHdrLen + len(payload)
	frame := make([]byte, totalLen)

	// Ethernet header
	frame[0], frame[1], frame[2], frame[3], frame[4], frame[5] = 0xff, 0xff, 0xff, 0xff, 0xff, 0xff
	frame[6] = 0x02
	binary.BigEndian.PutUint16(frame[12:14], 0x0800)

	// IPv4 header
	frame[14] = 0x45
	binary.BigEndian.PutUint16(frame[16:18], uint16(ipHdrLen+udpHdrLen+len(payload)))
	frame[24] = 64 // TTL
	frame[25] = 17 // UDP
	copy(frame[26:30], srcIP.To4())
	copy(frame[30:34], dstIP.To4())
	ipChk := checksum(frame[14:34])
	binary.BigEndian.PutUint16(frame[24:26], ipChk)

	// UDP header
	udpOffset := 14 + ipHdrLen
	binary.BigEndian.PutUint16(frame[udpOffset:udpOffset+2], srcPort)
	binary.BigEndian.PutUint16(frame[udpOffset+2:udpOffset+4], dstPort)
	binary.BigEndian.PutUint16(frame[udpOffset+4:udpOffset+6], uint16(udpHdrLen+len(payload)))
	copy(frame[udpOffset+8:], payload)

	udpChk := computeUDPChecksum(srcIP, dstIP, frame[udpOffset:])
	binary.BigEndian.PutUint16(frame[udpOffset+6:udpOffset+8], udpChk)

	b.mu.Lock()
	if !b.closed && b.tap != nil {
		_, _ = b.tap.Write(frame)
	}
	b.mu.Unlock()
}

func checksum(data []byte) uint16 {
	var sum uint32
	for i := 0; i < len(data)-1; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(data[i : i+2]))
	}
	if len(data)%2 == 1 {
		sum += uint32(data[len(data)-1]) << 8
	}
	for sum > 0xffff {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum)
}

func computeTCPChecksum(srcIP, dstIP net.IP, tcpHdrAndPayload []byte) uint16 {
	pseudo := make([]byte, 12+len(tcpHdrAndPayload))
	copy(pseudo[0:4], srcIP.To4())
	copy(pseudo[4:8], dstIP.To4())
	pseudo[8] = 0
	pseudo[9] = 6 // TCP
	binary.BigEndian.PutUint16(pseudo[10:12], uint16(len(tcpHdrAndPayload)))
	copy(pseudo[12:], tcpHdrAndPayload)
	return checksum(pseudo)
}

func computeUDPChecksum(srcIP, dstIP net.IP, udpHdrAndPayload []byte) uint16 {
	pseudo := make([]byte, 12+len(udpHdrAndPayload))
	copy(pseudo[0:4], srcIP.To4())
	copy(pseudo[4:8], dstIP.To4())
	pseudo[8] = 0
	pseudo[9] = 17 // UDP
	binary.BigEndian.PutUint16(pseudo[10:12], uint16(len(udpHdrAndPayload)))
	copy(pseudo[12:], udpHdrAndPayload)
	return checksum(pseudo)
}

func (b *BuiltinBackend) isIPAllowed(ip net.IP) bool {
	if b.pins == nil {
		return false
	}
	for _, domain := range b.cfg.AllowedDomains {
		for _, allowed := range b.pins.Get(domain) {
			if ip.Equal(allowed) {
				return true
			}
		}
	}
	return false
}

// Close terminates the TAP forwarder and releases resources.
func (b *BuiltinBackend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	if b.tap != nil {
		return b.tap.Close()
	}
	return nil
}

