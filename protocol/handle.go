package protocol

import (
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/lightclient/bazooka/attack"
)

func RunProtocol(pm *Manager, peer *p2p.Peer, rw p2p.MsgReadWriter) error {
	err := syncHandshake(pm.chain, rw)
	if err != nil {
		return fmt.Errorf("Handshake failed: %s", err)
	}

	syncComplete := false

	for {
		if syncComplete {
			for {
				r := <-pm.Routines
				exit, err := pm.handleRoutine(r, rw)
				if err != nil {
					return err
				}
				if exit != false {
					return nil
				}
			}
		}

		msg, err := rw.ReadMsg()
		if err != nil {
			return fmt.Errorf("failed to receive message from peer: %w", err)
		}

		switch {
		case msg.Code == eth.GetBlockHeadersMsg:
			if err = pm.handleGetBlockHeaderMsg(msg, rw); err != nil {
				return err
			}
		case msg.Code == eth.GetBlockBodiesMsg:
			if err = pm.handleGetBlockBodiesMsg(msg, rw); err != nil {
				return err
			}
		case msg.Code == eth.NewBlockHashesMsg:
			if syncComplete, err = pm.handleNewBlockHashesMsg(msg, rw); err != nil {
				return err
			}
		default:
			log.Trace("Unrecognized message", "msg", msg)
		}
	}
}

func (pm *Manager) handleGetBlockHeaderMsg(msg p2p.Msg, rw p2p.MsgReadWriter) error {
	var query getBlockHeadersData
	if err := msg.Decode(&query); err != nil {
		return fmt.Errorf("failed to decode msg %v: %w", msg, err)
	}

	log.Trace("GetBlockHeadersMsg", "query", query)

	if query.Reverse {
		return fmt.Errorf("reverse not supported")
	}

	headers := []*types.Header{}

	// if selecting via hash, convert to number
	if query.Origin.Hash != (common.Hash{}) {
		header := pm.chain.GetHeaderByHash(query.Origin.Hash)
		if header != nil {
			query.Origin.Hash = common.Hash{}
			query.Origin.Number = header.Number.Uint64()
		} else {
			return fmt.Errorf("Could not find header with hash %d\n", query.Origin.Hash)
		}
	}

	// find hashes via number
	number := query.Origin.Number
	for i := 0; i < int(query.Amount); i++ {
		if header := pm.chain.GetHeaderByNumber(number); header != nil {
			headers = append(headers, header)
		}
		number += query.Skip + 1
	}

	if err := p2p.Send(rw, eth.BlockHeadersMsg, headers); err != nil {
		return fmt.Errorf("failed to send headers: %w", err)
	}

	return nil
}

func (pm *Manager) handleGetBlockBodiesMsg(msg p2p.Msg, rw p2p.MsgReadWriter) error {
	log.Trace("GetBlockBodiesMsg")

	msgStream := rlp.NewStream(msg.Payload, uint64(msg.Size))
	if _, err := msgStream.List(); err != nil {
		return err
	}

	var (
		hash   common.Hash
		bytes  int
		bodies []rlp.RawValue
	)

	for {
		if err := msgStream.Decode(&hash); err == rlp.EOL {
			break
		} else if err != nil {
			return fmt.Errorf("msg %v: %v", msg, err)
		}

		if data := pm.chain.GetBodyRLP(hash); len(data) != 0 {
			bodies = append(bodies, data)
			bytes += len(data)
		}
	}

	if err := p2p.Send(rw, eth.BlockBodiesMsg, bodies); err != nil {
		return err
	}

	return nil
}

func (pm *Manager) handleNewBlockHashesMsg(msg p2p.Msg, rw p2p.MsgReadWriter) (bool, error) {
	var blockHashMsg newBlockHashesData
	if err := msg.Decode(&blockHashMsg); err != nil {
		return false, fmt.Errorf("failed to decode msg %v: %w", msg, err)
	}

	log.Trace("NewBlockHashesMsg", "query", blockHashMsg)

	syncComplete := false
	for _, bh := range blockHashMsg {
		if bh.Number == pm.chain.CurrentBlock().NumberU64() {
			syncComplete = true
			break
		}
	}
	return syncComplete, nil
}

func (pm *Manager) handleRoutine(r attack.Routine, rw p2p.MsgReadWriter) (bool, error) {
	switch r.Ty {
	case attack.SendTxs:
		log.Info("Sending new transaction msg", "txs", len(r.SignedTransactions))
		return false, p2p.Send(rw, eth.TransactionMsg, r.SignedTransactions)
	case attack.SendBlock:
		block := r.SignedBlock
		td := big.NewInt(1000)
		log.Info("Sending new block msg", "height", r.SignedBlock.Number())
		return false, p2p.Send(rw, eth.NewBlockMsg, []interface{}{&block, td})
	case attack.Sleep:
		log.Info("Sleeping", "time", r.SleepDuration)
		time.Sleep(r.SleepDuration)
		return false, nil
	case attack.Exit:
		log.Info("Exiting handler")
		os.Exit(0)
		return true, nil
	default:
		return false, fmt.Errorf("Unrecognized routine type")
	}
}
