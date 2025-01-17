package p2p

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/lightclient/bazooka/protocol"
)

func MakeP2PServer(pm *protocol.Manager) *p2p.Server {
	serverKey, err := crypto.GenerateKey()
	if err != nil {
		panic(fmt.Sprintf("Failed to generate server key: %v", err))
	}

	p2pConfig := p2p.Config{
		PrivateKey: serverKey,
		Name:       "bazooka",
		Logger:     log.New(),
		MaxPeers:   1,
		Protocols: []p2p.Protocol{
			p2p.Protocol{
				Name:    "eth",
				Version: 64,
				Length:  17,
				Run: func(peer *p2p.Peer, rw p2p.MsgReadWriter) error {
					return protocol.RunProtocol(pm, peer, rw)
				},
			},
		},
	}

	return &p2p.Server{Config: p2pConfig}
}

func AddLocalPeer(server *p2p.Server, targetDataDir string) error {
	node, err := GetTargetAddr(targetDataDir)
	if err != nil {
		return err
	}

	server.AddPeer(node)

	return nil
}

func GetTargetAddr(targetDataDir string) (*enode.Node, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	nodekeyDir := fmt.Sprintf("%s/%s/geth/nodekey", cwd, targetDataDir)
	nodeKeyHex, err := ioutil.ReadFile(nodekeyDir)
	if err != nil {
		return nil, err
	}

	nodeKey, err := crypto.HexToECDSA(string(nodeKeyHex))
	nodeid := fmt.Sprintf("%x", crypto.FromECDSAPub(&nodeKey.PublicKey)[1:])

	addr := fmt.Sprint("enode://", nodeid, "@127.0.0.1:30303?discport=0")
	fmt.Println(addr)

	nodeToConnect, err := enode.ParseV4(string(addr))
	if err != nil {
		return nil, fmt.Errorf("could not parse the node info: %w", err)
	}

	log.Info(fmt.Sprintf("Parsed node: %s, IP: %s\n", nodeToConnect, nodeToConnect.IP()))

	return nodeToConnect, nil
}
