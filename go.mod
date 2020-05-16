module github.com/likecoin/iscn-poc

go 1.13

replace github.com/ipfs/go-ipfs => ./go-ipfs

replace github.com/likecoin/iscn-ipld => ./go-ipfs/plugin/plugins/iscn-ipld

require (
	github.com/btcsuite/btcutil v0.0.0-20190425235716-9e5f4b9a998d
	github.com/cosmos/cosmos-sdk v0.38.1
	github.com/ethereum/go-ethereum v1.9.13 // indirect
	github.com/ipfs/go-block-format v0.0.2
	github.com/ipfs/go-cid v0.0.5
	github.com/ipfs/go-ipfs v0.5.0
	github.com/ipfs/go-ipfs-addr v0.0.1 // indirect
	github.com/ipfs/go-ipfs-config v0.5.3
	github.com/ipfs/interface-go-ipfs-core v0.2.7
	github.com/likecoin/iscn-ipld v0.0.0-00010101000000-000000000000
	github.com/tendermint/tendermint v0.33.0
	github.com/tidwall/pretty v1.0.1
)
