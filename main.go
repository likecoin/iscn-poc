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
	iscncontent "github.com/likecoin/iscn-ipld/plugin/block/content"
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

func testContent(ctx context.Context, ipfs icore.CoreAPI) iscn.IscnObject {
	// --------------------------------------------------
	log.Printf("Generating content v1 block ...")
	data := map[string]interface{}{
		"type":        "article",
		"version":     1,
		"parent":      nil,
		"source":      "https://example.com/index.html",
		"edition":     "v.0.1",
		"fingerprint": "hash://sha256/9f86d081884c7d659a2feaa0",
		"title":       "Hello World!!!",
		"description": "Just to say hello to world.",
		"tags":        []string{"hello", "world", "blog"},
	}

	b1, err := iscncontent.NewContentBlock(1, data)
	if err != nil {
		log.Panicf("Cannot create content v1 block: %s", err)
	}
	log.Printf("New content v1 block %s", b1.RawData())

	log.Printf("Generating content v2 block ...")
	data = map[string]interface{}{
		"type":        "article",
		"version":     2,
		"parent":      b1.Cid(),
		"fingerprint": "hash://sha256/9f86d081884c7d659a2feaa0",
		"title":       "Hello World!!!",
	}

	b2, err := iscncontent.NewContentBlock(1, data)
	if err != nil {
		log.Panicf("Cannot create content v2 block: %s", err)
	}
	log.Printf("New content v2 block %s", b2.RawData())

	// --------------------------------------------------
	log.Printf("Pinning content blocks ...")

	if err := ipfs.Dag().Pinning().Add(ctx, b1); err != nil {
		log.Panicf("Cannot pin IPLD: %s", err)
	}
	if err := ipfs.Dag().Pinning().Add(ctx, b2); err != nil {
		log.Panicf("Cannot pin IPLD: %s", err)
	}

	// --------------------------------------------------
	log.Printf("Getting content blocks ...")

	ret1, err := ipfs.Dag().Get(ctx, b1.Cid())
	if err != nil {
		log.Panicf("Cannot fetch IPLD: %s", err)
		return nil
	}

	d1, err := iscn.Decode(ret1.RawData(), b1.Cid())
	if err != nil {
		log.Panicf("Cannot decode IPLD raw data: %s", err)
	}

	obj1, ok := d1.(iscn.IscnObject)
	if !ok {
		log.Panicf("Cannot convert to \"IscnObject\" for content v1")
	}

	ret2, err := ipfs.Dag().Get(ctx, b2.Cid())
	if err != nil {
		log.Panicf("Cannot fetch IPLD: %s", err)
		return nil
	}

	d2, err := iscn.Decode(ret2.RawData(), b2.Cid())
	if err != nil {
		log.Panicf("Cannot decode IPLD raw data: %s", err)
	}

	obj2, ok := d2.(iscn.IscnObject)
	if !ok {
		log.Panicf("Cannot convert to \"IscnObject\" for content v2")
	}

	// --------------------------------------------------
	// Report
	log.Println("********************************************************************************")
	log.Println("Content report")
	log.Println("********************************************************************************")

	log.Printf("Content version 1")
	c1, err := b1.Cid().StringOfBase('z')
	if err != nil {
		log.Panicf("Cannot retrieve CID from block: %s", err)
	}
	log.Printf("  CID: %s", c1)

	log.Printf("  Raw data: %s", b1.RawData())

	log.Printf("  Type: %s", obj1.GetName())
	log.Printf("  Schema version: %d", obj1.GetVersion())

	if val, err := obj1.GetString("type"); ok {
		log.Printf("  Content type: %q", val)
	} else {
		log.Panicf("%s", err)
	}

	if val, err := obj1.GetUint64("version"); err == nil {
		log.Printf("  Version: %d", val)
	} else {
		log.Panicf("%s", err)
	}

	if _, err := obj1.GetCid("parent"); err == nil {
		log.Panic("Should not have property \"parent\"")
	}

	if val, err := obj1.GetString("source"); err == nil {
		log.Printf("  Source: %q", val)
	} else {
		log.Panicf("%s", err)
	}

	if val, err := obj1.GetString("edition"); err == nil {
		log.Printf("  Edition: %q", val)
	} else {
		log.Panicf("%s", err)
	}

	if val, err := obj1.GetString("fingerprint"); err == nil {
		log.Printf("  Fingerprint: %q", val)
	} else {
		log.Panicf("%s", err)
	}

	if val, err := obj1.GetString("title"); err == nil {
		log.Printf("  Title: %q", val)
	} else {
		log.Panicf("%s", err)
	}

	if val, err := obj1.GetString("description"); err == nil {
		log.Printf("  Description: %q", val)
	} else {
		log.Panicf("%s", err)
	}

	if val, err := obj1.GetArray("tags"); err == nil {
		log.Printf("  Tags: %v", val)
	} else {
		log.Panicf("%s", err)
	}

	log.Printf("Content version 2")
	c2, err := b2.Cid().StringOfBase('z')
	if err != nil {
		log.Panicf("Cannot retrieve CID from block: %s", err)
	}
	log.Printf("  CID: %s", c2)

	log.Printf("  Raw data: %s", b2.RawData())

	log.Printf("  Type: %s", obj2.GetName())
	log.Printf("  Schema version: %d", obj2.GetVersion())

	if val, err := obj2.GetString("type"); ok {
		log.Printf("  Content type: %q", val)
	} else {
		log.Panicf("%s", err)
	}

	if val, err := obj2.GetUint64("version"); err == nil {
		log.Printf("  Version: %d", val)
	} else {
		log.Panicf("%s", err)
	}

	if val, err := obj2.GetCid("parent"); err == nil {
		c, err := val.StringOfBase('z')
		if err != nil {
			log.Panicf("Cannot retrieve CID for parent block: %s", err)
		}
		log.Printf("  Parent: %s", c)
	} else {
		log.Panicf("%s", err)
	}

	if _, err := obj2.GetString("source"); err == nil {
		log.Panic("Should not have property \"source\"")
	}

	if _, err := obj2.GetString("edition"); err == nil {
		log.Panic("Should not have property \"edition\"")
	}

	if val, err := obj2.GetString("fingerprint"); err == nil {
		log.Printf("  Fingerprint: %q", val)
	} else {
		log.Panicf("%s", err)
	}

	if val, err := obj2.GetString("title"); err == nil {
		log.Printf("  Title: %q", val)
	} else {
		log.Panicf("%s", err)
	}

	if _, err := obj2.GetString("description"); err == nil {
		log.Panic("Should not have property \"description\"")
	}

	if _, err := obj2.GetArray("tags"); err == nil {
		log.Panic("Should not have property \"tags\"")
	}

	// --------------------------------------------------
	// JSON

	log.Println()
	log.Println("********************************************************************************")
	log.Println("Content JSON")
	log.Println("********************************************************************************")

	log.Printf("Content version 1")
	json1, err := obj1.MarshalJSON()
	if err != nil {
		log.Panicf("Cannot marshal JSON: %s", err)
	}
	log.Println(string(json1))
	log.Println(string(pretty.Pretty([]byte(json1))))

	log.Printf("Content version 2")
	json2, err := obj2.MarshalJSON()
	if err != nil {
		log.Panicf("Cannot marshal JSON: %s", err)
	}
	log.Println(string(json2))
	log.Println(string(pretty.Pretty([]byte(json2))))

	return b2
}

func testIscnKernel(
	ctx context.Context,
	ipfs icore.CoreAPI,
	content iscn.IscnObject,
) {
	// --------------------------------------------------
	log.Printf("Generating ISCN kernel block ...")

	id := make([]byte, 32)
	rand.Read(id)

	data := map[string]interface{}{
		"id":        id,
		"timestamp": "2020-01-01T12:34:56Z",
		"version":   1,
		"content":   content.Cid(),
		"zzz":       -987654321,
		"yyy":       []string{"abc", "def", "ghi"},
		"xxx":       []byte{'x', 'y', 'z'},
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
	}

	d, err := iscn.Decode(ret.RawData(), b.Cid())
	if err != nil {
		log.Panicf("Cannot decode IPLD raw data: %s", err)
	}

	obj, ok := d.(iscn.IscnObject)
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
	log.Printf("  Schema version: %d", obj.GetVersion())

	if val, err := obj.GetBytes("id"); err == nil {
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
	log.Println("ISCN kernel JSON")
	log.Println("********************************************************************************")

	json, err := obj.MarshalJSON()
	if err != nil {
		log.Panicf("Cannot marshal JSON: %s", err)
	}
	log.Println(string(json))
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

	content := testContent(ctx, ipfs)
	testIscnKernel(ctx, ipfs, content)

	cms.Commit()

	<-done
	log.Println("Close plugin")
	err = plugins.Close()
	if err != nil {
		log.Panicf("Cannot close plugins: %s", err)
	}
}
