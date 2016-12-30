package sniffer

import (
	"errors"
	"io"
	"log"
	"os"
	"time"

	"github.com/emfree/honeypacket/protocols"
	"github.com/google/gopacket"
	"github.com/google/gopacket/afpacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

type Sniffer struct {
	packetHandle packetDataSource
	iface        string
}

type packetDataSource interface {
	gopacket.PacketDataSource
	SetBPFFilter(filter string) error
}

type afpacketHandle struct {
	*afpacket.TPacket
}

type pcapHandle struct {
	*pcap.Handle
}

func (a *afpacketHandle) SetBPFFilter(filter string) error { return errors.New("not implemented") }

func New(iface string, bufferSizeMb int, snaplen int, pollTimeout time.Duration) (*Sniffer, error) {
	s := &Sniffer{iface: iface}
	if true {
		var err error
		if s.iface == "" {
			s.iface = "any"
		}
		s.packetHandle, err = newPcapHandle(s.iface, snaplen, pollTimeout)
		if err != nil {
			return nil, err
		}
	} else {
		// not currently supported -- make configurable once we add an implementation of SetBPFFilter
		frameSize, blockSize, numBlocks, err := afpacketComputeSize(bufferSizeMb, snaplen, os.Getpagesize())
		if err != nil {
			return nil, err
		}
		s.packetHandle, err = newAfpacketHandle(s.iface, frameSize, blockSize, numBlocks, pollTimeout)
		if err != nil {
			return nil, err
		}
	}

	return s, nil

}

func (sniffer *Sniffer) Run() error {
	var linkLayerType gopacket.LayerType
	if sniffer.iface == "any" {
		linkLayerType = layers.LayerTypeLinuxSLL
	} else {
		linkLayerType = layers.LayerTypeEthernet

	}
	var sll layers.LinuxSLL
	var eth layers.Ethernet
	var ip4 layers.IPv4
	var ip6 layers.IPv6
	var tcp layers.TCP
	var payload gopacket.Payload
	decoder := gopacket.NewDecodingLayerParser(linkLayerType, &sll, &eth, &ip4, &ip6, &tcp, &payload)
	decodedLayers := make([]gopacket.LayerType, 0, 10)
	for {
		packetData, _, err := sniffer.packetHandle.ReadPacketData()

		if err == io.EOF {
			log.Println("EOF") // debug -- better handle this
			break
		} else if err != nil {
			log.Println("Err:", err)
			continue
		}

		if len(packetData) == 0 {
			continue
		}

		err = decoder.DecodeLayers(packetData, &decodedLayers)

		if err != nil {
			log.Println("Err:", err)
			continue
		}

		packetInfo := protocols.PacketInfo{}

		packetInfo.Truncated = decoder.Truncated

		for _, typ := range decodedLayers {
			switch typ {
			case layers.LayerTypeIPv4:
				packetInfo.SrcIP, packetInfo.DstIP = ip4.SrcIP, ip4.DstIP
			case layers.LayerTypeIPv6:
				packetInfo.SrcIP, packetInfo.DstIP = ip6.SrcIP, ip6.DstIP
			case layers.LayerTypeTCP:
				packetInfo.SrcPort, packetInfo.DstPort = uint16(tcp.SrcPort), uint16(tcp.DstPort)
			case gopacket.LayerTypePayload:
				packetInfo.Data = payload
			default:
				log.Println("type", typ, eth)
			}
		}

		log.Println("packet info", packetInfo)
	}
	return nil
}

func (sniffer *Sniffer) SetBPFFilter(filter string) error {
	return sniffer.packetHandle.SetBPFFilter(filter)
}

func newPcapHandle(iface string, snaplen int, pollTimeout time.Duration) (*pcapHandle, error) {
	h, err := pcap.OpenLive(iface, int32(snaplen), true, pollTimeout)
	return &pcapHandle{h}, err
}

func newAfpacketHandle(iface string, frameSize int, blockSize int, numBlocks int,
	pollTimeout time.Duration) (*afpacketHandle, error) {
	h, err := afpacket.NewTPacket(
		afpacket.OptInterface(iface),
		afpacket.OptFrameSize(frameSize),
		afpacket.OptBlockSize(blockSize),
		afpacket.OptNumBlocks(numBlocks),
		afpacket.OptPollTimeout(pollTimeout),
		afpacket.OptTPacketVersion(afpacket.TPacketVersion2)) // work around https://github.com/google/gopacket/issues/80
	return &afpacketHandle{h}, err
}

func afpacketComputeSize(targetSizeMb int, snaplen int, pageSize int) (
	frameSize int, blockSize int, numBlocks int, err error) {
	if snaplen < pageSize {
		frameSize = pageSize / (pageSize / snaplen)
	} else {
		frameSize = (snaplen/pageSize + 1) * pageSize
	}
	blockSize = frameSize * afpacket.DefaultNumBlocks
	numBlocks = (targetSizeMb * 1024 * 1024) / blockSize
	if numBlocks == 0 {
		return 0, 0, 0, errors.New("Buffer size too small")
	}

	return frameSize, blockSize, numBlocks, nil
}