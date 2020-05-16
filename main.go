package main

import (
	"bytes"
	"context"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/btcsuite/btcutil/base58"
	"github.com/cosmos/cosmos-sdk/store"
	"github.com/ipfs/go-ipfs/core"
	"github.com/ipfs/go-ipfs/core/coreapi"
	"github.com/ipfs/go-ipfs/plugin/loader"
	"github.com/ipfs/go-ipfs/plugin/plugins/cosmosds"
	"github.com/ipfs/go-ipfs/repo/fsrepo"
	"github.com/tidwall/pretty"

	cosmos "github.com/cosmos/cosmos-sdk/types"
	config "github.com/ipfs/go-ipfs-config"
	libp2p "github.com/ipfs/go-ipfs/core/node/libp2p"
	icore "github.com/ipfs/interface-go-ipfs-core"
	iscn "github.com/likecoin/iscn-ipld/plugin/block"
	iscnkernel "github.com/likecoin/iscn-ipld/plugin/block/kernel"
	abci "github.com/tendermint/tendermint/abci/types"
	tlog "github.com/tendermint/tendermint/libs/log"
)

func setupCosmosStore(plugins *loader.PluginLoader) cosmos.CommitMultiStore {
	pl, err := plugins.GetPlugin("ds-cosmos")
	if err != nil {
		log.Panicf("Cannot retrieve \"ds-cosmos\" plugin: %s", err)
	}

	cosmosDSPlugin, ok := pl.(*cosmosds.Plugin)
	if !ok {
		log.Panic("The plugin is not a \"*cosmosds.Plugin\"")
	}

	dataDir := filepath.Join(".", "cosmos")
	db, err := cosmos.NewLevelDB("application", dataDir)
	if err != nil {
		log.Panicf("Failed to create LevelDB: %s", err)
	}

	key := cosmos.NewKVStoreKey("StoreKey")
	cms := store.NewCommitMultiStore(db)
	cms.MountStoreWithDB(key, cosmos.StoreTypeIAVL, db)
	cms.LoadLatestVersion()
	ctx := cosmos.NewContext(cms, abci.Header{}, false, tlog.NewNopLogger())
	kv := ctx.KVStore(key)

	err = cosmosDSPlugin.SetCosmosStore(kv)
	if err != nil {
		log.Panicf("Cannot setup Cosmos store: %s", err)
	}

	return cms
}

func setupDefaultDatastoreConfig(cfg *config.Config) *config.Config {
	cfg.Datastore.Spec = map[string]interface{}{
		"mountpoint": "/",
		"type":       "measure",
		"prefix":     "cosmossdk.datastore",
		"child": map[string]interface{}{
			"type":        "cosmosds",
			"path":        "datastore",
			"compression": "none",
		},
	}
	return cfg
}

func setupNode(ctx context.Context) (
	*loader.PluginLoader,
	icore.CoreAPI,
	cosmos.CommitMultiStore,
	error) {
	rootPath, err := filepath.Abs("./ipfs")
	if err != nil {
		log.Printf("Cannot parse path: %s", err)
		return nil, nil, nil, err
	}

	plugins, err := loader.NewPluginLoader(rootPath)
	if err != nil {
		log.Printf("error loading plugins: %s", err)
		return nil, nil, nil, err
	}

	if err := plugins.Initialize(); err != nil {
		log.Printf("error initializing plugins: %s", err)
		return nil, nil, nil, err
	}

	if err := plugins.Inject(); err != nil {
		log.Printf("error initializing plugins: %s", err)
		return nil, nil, nil, err
	}

	rootPath, err = config.Path(rootPath, "")
	if err != nil {
		log.Printf("Cannot set config path: %s", err)
		return nil, nil, nil, err
	}

	cfg, err := config.Init(ioutil.Discard, 2048)
	if err != nil {
		log.Printf("Cannot init config: %s", err)
		return nil, nil, nil, err
	}
	cfg = setupDefaultDatastoreConfig(cfg)

	err = fsrepo.Init(rootPath, cfg)
	if err != nil {
		log.Printf("Cannot init repo: %s", "ab")
		return nil, nil, nil, err
	}

	repo, err := fsrepo.Open(rootPath)
	if err != nil {
		log.Printf("Cannot open repo: %s", err)
		return nil, nil, nil, err
	}

	nodeOptions := &core.BuildCfg{
		Online:  true,
		Routing: libp2p.DHTOption,
		Repo:    repo,
	}

	node, err := core.NewNode(ctx, nodeOptions)
	if err != nil {
		log.Printf("Cannot create node: %s", err)
		return nil, nil, nil, err
	}
	node.IsDaemon = true

	ipfs, err := coreapi.NewCoreAPI(node)
	if err != nil {
		log.Println("No IPFS repo available on the default path")
		return nil, nil, nil, err
	}

	log.Println("Start plugin")
	err = plugins.Start(node)
	if err != nil {
		log.Printf("Cannot start plugins: %s", err)
		return nil, nil, nil, err
	}

	cms := setupCosmosStore(plugins)

	return plugins, ipfs, cms, nil
}

func testIscnKernel(ctx context.Context, ipfs icore.CoreAPI) {
	// --------------------------------------------------
	log.Printf("Generating ISCN kernel block ...")

	id := make([]byte, 32)
	rand.Read(id)

	data := map[string]interface{}{
		"id":  id,
		"zzz": -987654321,
		"yyy": []string{"abc", "def", "ghi"},
		"xxx": []byte{'x', 'y', 'z'},
		"p": map[string]interface{}{
			"a": 10,
			"b": map[string]interface{}{
				"ba": "abc",
				"bb": 123,
			},
		},
	}

	b, err := iscnkernel.NewIscnKernelBlock(1, data)
	if err != nil {
		log.Panicf("Cannot create ISCN kernel block: %s", err)
	}

	// --------------------------------------------------
	log.Printf("Pinning ISCN kernel block ...")

	if err := ipfs.Dag().Pinning().Add(ctx, b); err != nil {
		log.Panicf("Cannot pin IPLD: %s", err)
	}

	// --------------------------------------------------
	log.Printf("Getting ISCN kernel block ...")

	ret, err := ipfs.Dag().Get(ctx, b.Cid())
	if err != nil {
		log.Panicf("Cannot fetch IPLD: %s", err)
		return
	}

	obj, ok := ret.(iscn.IscnObject)
	if !ok {
		log.Panicf("Cannot convert to \"IscnObject\"")
	}

	// --------------------------------------------------
	// Report
	log.Println("********************************************************************************")
	log.Println("ISCN kernel report")
	log.Println("********************************************************************************")

	c, err := b.Cid().StringOfBase('z')
	if err != nil {
		log.Panicf("Cannot retrieve CID from block: %s", err)
	}
	log.Printf("  CID: %s", c)

	log.Printf("  Raw data: %s", b.RawData())

	log.Printf("  Type: %s", obj.GetName())
	log.Printf("  Version: %d", obj.GetVersion())

	if val, err := obj.GetBytes("id"); ok {
		if !bytes.Equal(id, val) {
			log.Panic("ID is not matched")
		}

		log.Printf("  ID (original): %s", base58.Encode(id))
		log.Printf("  ID           : %s", base58.Encode(val))
	} else {
		log.Panicf("%s", err)
	}

	log.Println("  Custom properties:")
	for key, value := range obj.GetCustom() {
		log.Printf("    \"%s\":", key)
		log.Printf("      %T -> %v", value, value)
	}

	// --------------------------------------------------
	// JSON

	log.Println()
	log.Println("********************************************************************************")
	log.Println("ISCN kernel report")
	log.Println("********************************************************************************")

	json, err := obj.ToJSON()
	if err != nil {
		log.Panicf("Cannot marshal JSON: %s", err)
	}
	log.Println(json)
	log.Println(string(pretty.Pretty([]byte(json))))
}

func main() {
	rand.Seed(time.Now().UnixNano())

	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)

	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		log.Printf("\nGet signal: \"%v\"", sig)
		done <- true
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log.Println("Setting up IPFS node ...")
	plugins, ipfs, cms, err := setupNode(ctx)
	if err != nil {
		log.Panicf("Failed to set up IPFS node")
	}
	log.Println("IPFS node is created")

	testIscnKernel(ctx, ipfs)

	cms.Commit()

	<-done
	log.Println("Close plugin")
	err = plugins.Close()
	if err != nil {
		log.Panicf("Cannot close plugins: %s", err)
	}
}
