module github.com/likecoin/likecoin-iscn-poc

go 1.13

replace github.com/ipfs/go-ipfs => ./go-ipfs

replace github.com/likecoin/likecoin-iscn-ipld => ./go-ipfs/plugin/plugins/likecoin-iscn-ipld

require (
	github.com/ipfs/go-ipfs v0.4.23
	github.com/ipfs/go-ipfs-config v0.0.3
	github.com/ipfs/interface-go-ipfs-core v0.0.8
	go.uber.org/dig v1.8.0 // indirect
	go.uber.org/multierr v1.4.0 // indirect
)
