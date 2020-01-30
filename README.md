# likecoin-iscn-poc

PoC on combining LikeCoin chain and IPLD to serve as ISCN.

## Proof of concept

International Standard Content Number (ISCN) is a universal content registry. Before we try to implement the ISCN, we want to serve it on the LikeCoin chain which is based on [Cosmos SDK](https://cosmos.network/sdk) and at the same time, we also want to serve it on [InterPlanetary File System (IPFS)](https://ipfs.io/) as an [InterPlanetary Linked Data (IPLD)](https://ipld.io/) so that everyone can access it easily.

The concept is that we embed the IPFS as a library into LikeCoin chain, implement an [IPLD plugin](https://github.com/ipfs/go-ipfs/blob/master/plugin/ipld.go) to handle the ISCN data in [Concise Binary Object Representation (CBOR)](https://en.wikipedia.org/wiki/CBORhttps://en.wikipedia.org/wiki/CBOR) format and implement a [datastore plugin](https://github.com/ipfs/go-ipfs/blob/master/plugin/datastore.go) to store the ISCN data in Cosmos SDK store as part of chain data.