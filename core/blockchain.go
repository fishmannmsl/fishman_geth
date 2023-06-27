package core

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	log "github.com/sirupsen/logrus"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"zzschain/wallet"

	"github.com/boltdb/bolt"
	"github.com/ethereum/go-ethereum/common"
)

var mutex = &sync.Mutex{}

//Blockchain 区块链结构
//我们不在里面存储所有的区块了，而是仅存储区块链的 tip。
//另外，我们存储了一个数据库连接。因为我们想要一旦打开它的话，就让它一直运行，直到程序运行结束。
type Blockchain struct {
	Tip      []byte   //区块链最后一块的哈希值
	Database *bolt.DB //数据库
}

//MineBlock 挖出普通区块并将新区块加入到区块链中
//此方法通过区块链的指针调用，将修改区块链bc的内容
func (bc *Blockchain) MineBlock(transactions []*Transaction, coinbase string) *Block {
	//在将交易放入块之前进行签名验证
	for _, tx := range transactions {
		if bc.VerifyTransaction(tx) != true {
			log.Panic("ERROR: 非法交易")
		}
	}

	coinba := []byte(coinbase)

	newBlock := NewBlock(transactions, bc.Database, coinba) //挖出区块
	err := bc.Database.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(BlocksBucket))
		err := b.Put(newBlock.Hash.Bytes(), newBlock.Serialize()) //将新区块序列化后插入到数据库表中
		if err != nil {
			log.Panic(err)
		}

		err = b.Put([]byte("last"), newBlock.Hash.Bytes()) //更新区块链最后一个区块的哈希到数据库中
		Handle(err)
		bc.Tip = newBlock.Hash.Bytes() //修改区块链实例的tip值
		fmt.Println("区块hash：", Encode(bc.Tip[:]))
		return nil
	})
	Handle(err)

	return newBlock
}

// CreatBlockchain 创建一个全新的区块链数据库
// address用户发起创始交易，并挖矿，奖励也发给用户address
// 注意，创建后，数据库是open状态，需要使用者负责close数据库
func CreatBlockchain(address []byte, nodeId string) *Blockchain {
	var tip Hash //存储最后一块的哈希
	path := GetDatabasePath(nodeId)
	if DbExist(path) {
		fmt.Println("区块链已经存在")
		os.Exit(1)
	}
	db, err := OpenBoltDB(nodeId)
	Handle(err)

	err = db.Update(func(tx *bolt.Tx) error { //更新数据库，通过事务进行操作。一个数据文件同时只支持一个读-写事务
		genesisCoinbaseData := string(time.Now().Unix()) + ":创世块生成--"
		cbtx := NewCoinbaseTX(address, genesisCoinbaseData) //创建创始交易
		genesis := NewGenesisBlock(cbtx, address)           //创建创始区块

		b, err := tx.CreateBucket([]byte(BlocksBucket))
		Handle(err)

		d := genesis.Serialize()
		err = b.Put(genesis.Hash.Bytes(), d) //将创始区块序列化后插入到数据库表中
		Handle(err)

		//插入Tip到数据库，没有用到事务
		err = b.Put([]byte("last"), genesis.Hash.Bytes())
		Handle(err)
		tip = genesis.Hash

		return nil
	})
	Handle(err)

	BC := Blockchain{tip.Bytes(), db} //构建区块链实例

	return &BC //返回区块链实例的指针
}

//FindUnspentTransaction 查找未花费的交易（即该交易的花费尚未花出，换句话说，
//及该交易的输出尚未被其他交易作为输入包含进去）
func (bc *Blockchain) FindUnspentTransaction(pubKeyHash []byte) []Transaction {
	var unspentTXs []Transaction //未花费交易

	//已花费输出，key是转化为字符串的当前交易的ID
	//value是该交易包含的引用输出的所有已花费输出值数组
	//一个交易可能有多个输出，在这种情况下，该交易将引用所有的输出：输出不可分规则，无法引用它的一部分，要么不用，要么一次性用完
	//在go中，映射的值可以是数组，所以这里创建一个映射来存储未花费输出
	spentTXOs := make(map[string][]int)

	//从区块链中取得所有已花费输出
	bci := bc.Iterator()
	for { //第一层循环，对区块链中的所有区块进行迭代查询
		block := bci.Next()

		for _, tx := range block.Transactions { //第二层循环，对单个区块中的所有交易进行循环：一个区块可能打包了多个交易
			//检查交易的输入，将所有可以解锁的引用的输出加入到已花费输出map中
			if tx.IsCoinbase() == false { //不适用于创始区块的交易，因为它没有引用输输入
				for _, in := range tx.Vin { //第三层循环，对单个交易中的所有输入进行循环（一个交易可能有多个输入）
					if in.UsesKey(pubKeyHash) { //可以被pubKeyHash解锁，即属于pubKeyHash发起的交易（sender）
						inTxID := hex.EncodeToString(in.Txid)
						//in.Vout为引用输出在该交易所有输出中的一个索引
						spentTXOs[inTxID] = append(spentTXOs[inTxID], in.Vout) //加入到已花费映射之中
					}
				}
			}
		}
		if IsInitBlock(block.PrevHash.Bytes()) { //创始区块都检查完了，退出最外层循环
			break
		}
	}

	//获得未花费交易
	bci = bc.Iterator()
	for { //第一层循环，对区块链中的所有区块进行迭代查询
		block := bci.Next()

		for _, tx := range block.Transactions { //第二层循环，对单个区块中的所有交易进行循环：一个区块可能打包了多个交易
			txID := hex.EncodeToString(tx.ID) //交易ID转为字符串，便于比较

		Outputs:
			for outIdx, out := range tx.Vout { //第三层循环，对单个交易中的所有输出进行循环（一个交易可能有多个输出）
				//检查交易的输出，OutIdx为数组序号，实际上也是某个TxOutput的索引，out为TxOutput
				//一个交易，可能会有多个输出
				//输出是否已经花费了？
				if spentTXOs[txID] != nil {
					for _, spentOut := range spentTXOs[txID] { //第四层循环，对前面获得的所有未花费输出进行循环，spentOut是value
						//根据输出引用不可再分规则，
						//只要有一个输出值被引用，那么该输出的所有值都被引用了
						//所以通过比较索引值，只要发现一个输出值被引用了，就不必查询下一个输出值了
						//说明该输出已经被引用（被包含在其它交易的输入之中，即被花费掉了）
						if spentOut == outIdx {
							continue Outputs //在 continue 语句后添加标签Outputs时，表示开始标签Outputs对应的循环
						}
					}
				}

				//输出没有被花费，且由pubKeyHash锁定（即归pubKeyHash用户所有）
				if out.IsLockedWithKey(pubKeyHash) {
					unspentTXs = append(unspentTXs, *tx) //将tx值加入到已花费交易数组中
				}
			}

		}
		if IsInitBlock(block.PrevHash.Bytes()) { //创始区块都检查完了，退出最外层循环
			break
		}
	}
	return unspentTXs
}

//FindUTXO 从区块链中取得所有未花费输出及包含未花费输出的block
//只会区块链新创建后调用一次，其他时候不会调用
//不再需要调用者的公钥，因为我们保存到bucket的UTXO是所有的未花费输出
func (bc *Blockchain) FindUTXO() (map[string]TxOutputs, map[string]common.Hash) {
	UTXO := make(map[string]TxOutputs)        //未花费输出
	UTXOBlock := make(map[string]common.Hash) //含有未花费输出的block（只保存tx.ID-block.Hash)
	spentTXOs := make(map[string][]int)       // 已花费输出

	bci := bc.Iterator()

	for {
		block := bci.Next() //迭代区块链

		for _, tx := range block.Transactions { //迭代block的交易组
			txID := hex.EncodeToString(tx.ID)

		Outputs:
			for outIdx, out := range tx.Vout {
				// tx的输出是否已经花费
				if spentTXOs[txID] != nil {
					for _, spentOutIdx := range spentTXOs[txID] {
						if spentOutIdx == outIdx {
							outs := make([]TxOutput, 0)  //创建切片，长度为0，out实际上是一个空数组
							UTXO[txID] = TxOutputs{outs} //UTXO[txID]为空
							break Outputs                //返回到Outputs标识处，即退出循环for outIdx, out := range tx.Vout
						}
					}
				}

				//spentTXOs[txID] == nil
				outs := UTXO[txID]
				outs.Outputs = append(outs.Outputs, out)
				UTXO[txID] = outs
			}

			//SpentTXOs:
			if tx.IsCoinbase() == false {
				for _, in := range tx.Vin {
					inTxID := hex.EncodeToString(in.Txid)
					spentTXOs[inTxID] = append(spentTXOs[inTxID], in.Vout)
				}
			}
			UTXOBlock[txID] = common.BytesToHash(block.Hash.Bytes())
		}

		if IsInitBlock(block.PrevHash.Bytes()) {
			break
		}
	}

	return UTXO, UTXOBlock
}

// NewBlockchain 从数据库中取出最后一个区块的哈希，构建一个区块链实例
func NewBlockchain(nodeID string) *Blockchain {
	dbFile := GetDatabasePath(nodeID)
	if DbExist(dbFile) == false {
		sourceDb := GetDatabasePath("")
		fmt.Println("该端口区块链文件不存在，拷贝创世链文件")
		CPInitToNode(sourceDb, dbFile)
		fmt.Printf("成功创建文件:%s\n", dbFile)
	}

	var tip []byte
	db, err := bolt.Open(dbFile, 0600, nil)

	if err != nil {
		log.Panic(err)
	}
	err = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(BlocksBucket)) //通过名称获得bucket
		tip = b.Get([]byte("last"))          //获得最后区块的哈希

		return nil
	})
	if err != nil {
		log.Panic(err)
	}

	bc := Blockchain{Tip: tip, Database: db}

	return &bc
}

// AddBlock 将区块加入到本地区块链中
func (bc *Blockchain) AddBlock(block *Block) {
	mutex.Lock()

	err := bc.Database.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(BlocksBucket))
		blockInDb := b.Get(block.Hash.Bytes())

		if blockInDb != nil { //如果区块已经存在于数据库，退出
			bid := DeserializeBlock(blockInDb)
			fmt.Println("block in db hash:")
			fmt.Println(hex.EncodeToString(bid.Hash.Bytes()))
			fmt.Println(bc.Database.Path())
			return nil
		}

		fmt.Println("start put the block into database...")
		blockData := block.Serialize()
		err := b.Put(block.Hash.Bytes(), blockData)
		Handle(err)

		lastHash := b.Get([]byte("last"))
		lastBlockData := b.Get(lastHash)
		lastBlock := DeserializeBlock(lastBlockData)

		if block.Number.Cmp(lastBlock.Number) > 0 {
			err = b.Put([]byte("last"), block.Hash.Bytes())
			Handle(err)
			bc.Tip = block.Hash.Bytes()
		}
		fmt.Println("finished！")
		return nil
	})
	Handle(err)
	mutex.Unlock()
}

// GetBestNumber 返回最后一个区块号
func (bc *Blockchain) GetBestNumber() *big.Int {
	var lastBlock Block

	err := bc.Database.View(func(tx *bolt.Tx) error { //只读打开，读取最后一个区块的哈希，作为新区块的prevHash
		b := tx.Bucket([]byte(BlocksBucket))
		lastHash := b.Get([]byte("last")) //最后一个区块的哈希的键是字符串"1"
		blockData := b.Get(lastHash)
		lastBlock = *DeserializeBlock(blockData)
		return nil
	})

	Handle(err)

	return lastBlock.Number
}

// GetBlock 通过哈希返回一个区块
func (bc *Blockchain) GetBlock(blockHash []byte) (Block, error) {
	var block Block

	err := bc.Database.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(BlocksBucket))

		blockData := b.Get(blockHash)

		if blockData == nil {
			return errors.New("没有找到区块。")
		}

		block = *DeserializeBlock(blockData)

		return nil
	})
	if err != nil {
		return block, err
	}

	return block, nil
}

// GetBlockHashes 返回区块链中所有区块哈希列表
func (bc *Blockchain) GetBlockHashes() [][]byte {
	var blocks [][]byte
	bci := bc.Iterator()

	for {
		block := bci.Next()
		prevHash := block.PrevHash
		blocks = append([][]byte{block.Hash.Bytes()}, blocks...)

		if IsInitBlock(prevHash.Bytes()) {
			log.Info("该区块为创世块，所有哈希列表已全部返回")
			break
		}
	}

	return blocks
}

// GetTransationHashes 返回区块链中所有区块哈希列表
func (bc *Blockchain) GetTransationHashes() []Transaction {
	var transation []Transaction
	bci := bc.Iterator()

	for {
		block := bci.Next()
		for _, value := range block.Transactions {
			transation = append([]Transaction{*value}, transation...)
		}
		prevHash := block.PrevHash

		if IsInitBlock(prevHash.Bytes()) {
			log.Info("该区块为创世块，所有交易哈希列表已全部返回")
			break
		}
	}

	return transation
}

// GetBlockByNumber 返回区块链中的指定Number的区块指针
func (bc *Blockchain) GetBlockByNumber(number *big.Int) *Block {
	var block *Block
	bci := bc.Iterator()

	for {
		b := bci.Next()
		prevHash := b.PrevHash
		if b.Number.Cmp(number) == 0 {
			block = b
			break
		}

		if IsInitBlock(prevHash.Bytes()) {
			log.Info("该区块号(Number)不存在")
			break
		}
	}

	return block
}

//FindSpendableOutput 查找某个用户可以花费的输出，放到一个映射里面
//从未花费交易里取出未花费的输出，直至取出输出的币总数大于或等于需要send的币数为止
func (bc *Blockchain) FindSpendableOutput(pubKeyHash []byte, amount int) (int, map[string][]int) {
	unpsentOutputs := make(map[string][]int)
	unspentTXs := bc.FindUnspentTransaction(pubKeyHash)
	accumulated := 0 //sender发出的转出的全部币数

Work:
	for _, tx := range unspentTXs {
		txID := hex.EncodeToString(tx.ID)

		for outIdx, out := range tx.Vout {
			if out.IsLockedWithKey(pubKeyHash) && accumulated < amount {
				accumulated += out.Value
				unpsentOutputs[txID] = append(unpsentOutputs[txID], outIdx)

				if accumulated >= amount {
					break Work
				}
			}
		}
	}

	return accumulated, unpsentOutputs
}

// FindTransactionForUTXO 根据交易ID查询到一个交易，仅仅查询UTXOBlock的数据库，不需要迭代整个区块链
func (bc *Blockchain) FindTransactionForUTXO(txID []byte) (Transaction, error) {
	var tnx Transaction
	err := bc.Database.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(BlocksBucket))
		blockb := tx.Bucket([]byte(utxoBlockBucket)) //UTXOBlock

		blockhash := blockb.Get(txID) //UTXOBlock
		blockData := b.Get(blockhash)
		block := *DeserializeBlock(blockData)
		for _, tx := range block.Transactions {
			if bytes.Compare(tx.ID, txID) == 0 {
				tnx = *tx
			}
		}
		return nil
	})
	if err != nil {
		log.Panic(err)
	}
	if &tnx != nil {
		return tnx, nil
	}

	return Transaction{}, errors.New("未找到交易")
}

// FindTransaction 迭代整个区块链，根据交易ID查询到一个交易
func (bc *Blockchain) FindTransaction(txID []byte) (Transaction, error) {
	bci := bc.Iterator()

	for {
		block := bci.Next()

		for _, tx := range block.Transactions {
			if bytes.Compare(tx.ID, txID) == 0 {
				return *tx, nil
			}
		}

		if IsInitBlock(block.PrevHash.Bytes()) {
			break
		}
	}

	return Transaction{}, errors.New("未找到交易")
}

// SignTransaction 对一个交易的所有输入引用的输出的交易进行签名
//注意，这里签名的不是参数tx（当前交易），而是tx输入所引用的输出的交易
func (bc *Blockchain) SignTransaction(tx *Transaction, privKey ecdsa.PrivateKey) {
	prevTXs := make(map[string]Transaction)

	for _, vin := range tx.Vin {
		prevTX, err := bc.FindTransactionForUTXO(vin.Txid) //通过交易输入引用的输出交易ID获得输出交易
		if err != nil {
			log.Panic(err)
		}
		prevTXs[hex.EncodeToString(prevTX.ID)] = prevTX
	}

	tx.Sign(privKey, prevTXs)
}

// VerifyTransaction 验证一个交易的所有输入的签名
func (bc *Blockchain) VerifyTransaction(tx *Transaction) bool {
	if tx.IsCoinbase() {
		return true
	}

	prevTXs := make(map[string]Transaction)

	for _, vin := range tx.Vin {
		prevTX, err := bc.FindTransactionForUTXO(vin.Txid)
		if err != nil {
			log.Panic(err)
		}
		prevTXs[hex.EncodeToString(prevTX.ID)] = prevTX
	}

	return tx.Verify(prevTXs)
}

//Rount() 创建多复用路由
func (bc *Blockchain) Rount() *http.ServeMux {
	// 创建路由器
	mux := http.NewServeMux()

	//通过区块号查询区块
	mux.HandleFunc("/", Hello)
	//通过区块号查询区块
	mux.HandleFunc("/blockbynumber", bc.blockbynumber)
	//通过区块哈希查询区块
	mux.HandleFunc("/blockbyhash", bc.blockbyhash)
	//转账
	mux.HandleFunc("/sendtransation", bc.send)
	//交易查询
	mux.HandleFunc("/transationtohash", bc.gettransation)
	//查询余额
	mux.HandleFunc("/getbalance", bc.getbalance)
	//添加新钱包
	mux.HandleFunc("/addwallet", bc.addwallet)
	//加载钱包
	mux.HandleFunc("/getwallet", bc.getwallet)
	//显示历史交易
	mux.HandleFunc("/gethistory", bc.gethistory)
	return mux
}

type Blo struct {
	Number    string `json:"number"`
	BlockHash string `json:"hash"`
}

func (bc *Blockchain) blockbynumber(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		http.Error(w, "Unsupported Media Type", http.StatusUnsupportedMediaType)
		return
	}
	var blo Blo
	err := json.NewDecoder(r.Body).Decode(&blo)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	num, _ := strconv.Atoi(blo.Number)
	numb := big.NewInt(int64(num))
	bloc := bc.GetBlockByNumber(numb)
	jsonData, err := json.Marshal(bloc)
	w.Header().Set("Content-Type", "application/json")

	_, err = w.Write(jsonData)
	if err != nil {
		log.Println(err)
	}
}

//通过哈希获取block
func (bc *Blockchain) blockbyhash(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		http.Error(w, "Unsupported Media Type", http.StatusUnsupportedMediaType)
		return
	}
	var blo Blo
	err := json.NewDecoder(r.Body).Decode(&blo)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	blocHash, err := Decode(blo.BlockHash)
	if err != nil {
		panic(nil)
	}
	bloc, err := bc.GetBlock(blocHash)
	if err != nil {
		panic(err)
	}
	jsonData, err := json.Marshal(bloc)
	w.Header().Set("Content-Type", "application/json")

	_, err = w.Write(jsonData)
	if err != nil {
		log.Println(err)
	}
}

type Send struct {
	Sender string `json:"sender_blockchain_address"`
	Recip  string `json:"recipient_blockchain_address"`
	Value  string `json:"value"`
}

type Resp struct {
	Message string `json:"message"`
}

//转账
func (bc *Blockchain) send(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		http.Error(w, "Unsupported Media Type", http.StatusUnsupportedMediaType)
		return
	}
	// 获取请求的完整主机地址
	host := r.Host
	nodeId := ""
	// 使用 ":" 进行分割，提取出端口信息
	parts := strings.Split(host, ":")
	if len(parts) > 1 {
		nodeId = parts[1]
	} else {
		fmt.Println("无法获取请求的端口")
	}
	var tra Send
	err := json.NewDecoder(r.Body).Decode(&tra)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !wallet.ValidateAddress(tra.Sender) {
		log.Panic("ERROR: 发送地址非法")
	}
	if !wallet.ValidateAddress(tra.Recip) {
		log.Panic("ERROR: 接收地址非法")
	}
	UTXOSet := UTXOSet{bc}
	wallets, err := wallet.NewWallets(nodeId)
	if err != nil {
		log.Panic(err)
	}
	wallet := wallets.GetWallet(tra.Sender)
	value, _ := strconv.Atoi(tra.Value)
	tx := NewUTXOTransaction(&wallet, []byte(tra.Recip), value, &UTXOSet)
	//当前是挖矿节点，有奖励
	cbTx := NewCoinbaseTX([]byte(tra.Sender), "")
	txs := []*Transaction{cbTx, tx}

	newBlock := bc.MineBlock(txs, tra.Sender)
	UTXOSet.Update(newBlock)
	if err != nil {
		panic(err)
	}
	result := Resp{
		Message: "成功转账给：" + tra.Recip + "：" + tra.Value,
	}
	jsonData, err := json.Marshal(result)
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(jsonData)
	if err != nil {
		log.Println(err)
	}
}

type Tra struct {
	Trans string `json:"hash"`
}

func (bc *Blockchain) gettransation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		http.Error(w, "Unsupported Media Type", http.StatusUnsupportedMediaType)
		return
	}
	var tra Tra
	err := json.NewDecoder(r.Body).Decode(&tra)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	traHash, err := Decode(tra.Trans)
	if err != nil {
		panic(nil)
	}
	bloc, err := bc.FindTransaction(traHash)
	if err != nil {
		panic(err)
	}
	jsonData, err := json.Marshal(bloc)
	w.Header().Set("Content-Type", "application/json")

	_, err = w.Write(jsonData)
	if err != nil {
		log.Println(err)
	}

}

type WalletAddress struct {
	Blockchainaddress string `json:"blockchain_address"`
}

type Balance struct {
	Balance int `json:"balance"`
}

func (bc *Blockchain) getbalance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		http.Error(w, "Unsupported Media Type", http.StatusUnsupportedMediaType)
		return
	}
	var addr WalletAddress
	err := json.NewDecoder(r.Body).Decode(&addr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !wallet.ValidateAddress(addr.Blockchainaddress) {
		log.Panic("ERROR: 地址非法")
	}
	balance := 0
	pubKeyHash := wallet.Base58Decode([]byte(addr.Blockchainaddress))
	pubKeyHash = pubKeyHash[1 : len(pubKeyHash)-4]

	UTXOSet := UTXOSet{bc}
	UTXOs := UTXOSet.FindUTXO(pubKeyHash)

	for _, output := range UTXOs {
		balance += output.Value
	}

	fmt.Printf("'%s'的账号余额是: %d\n", addr.Blockchainaddress, balance)
	result := Balance{
		Balance: balance,
	}
	jsonData, err := json.Marshal(result)
	w.Header().Set("Content-Type", "application/json")

	_, err = w.Write(jsonData)
	if err != nil {
		log.Println(err)
	}
}
func (bc *Blockchain) addwallet(w http.ResponseWriter, r *http.Request) {
	// 获取请求的完整主机地址
	host := r.Host
	nodeId := ""
	// 使用 ":" 进行分割，提取出端口信息
	parts := strings.Split(host, ":")
	if len(parts) > 1 {
		nodeId = parts[1]
	} else {
		fmt.Println("无法获取请求的端口")
	}
	wallets, _ := wallet.NewWallets(nodeId) //从钱包文件读取所有的钱包
	address := wallets.CreateWallet()       //创建新钱包
	wallets.SaveToFile(nodeId)              //创建完成后，保存到本地，不参与网络共享，必须自己保管好！

	private := wallets.Wallets[address].PrivateKey.D.Bytes()
	privateStr := hex.EncodeToString(private)
	publicStr := hex.EncodeToString(wallets.Wallets[address].PublicKey)

	result := Wal{
		privateStr,
		address,
		publicStr,
	}

	jsonData, err := json.Marshal(result)
	w.Header().Set("Content-Type", "application/json")

	_, err = w.Write(jsonData)
	if err != nil {
		log.Println(err)
	}
}

type Pri struct {
	Privatekey string `json:"privatekey"`
}

func (bc *Blockchain) getwallet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	// 获取请求的完整主机地址
	host := r.Host
	nodeId := ""
	// 使用 ":" 进行分割，提取出端口信息
	parts := strings.Split(host, ":")
	if len(parts) > 1 {
		nodeId = parts[1]
	} else {
		fmt.Println("无法获取请求的端口")
	}
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		http.Error(w, "Unsupported Media Type", http.StatusUnsupportedMediaType)
		return
	}
	var priv Pri
	err := json.NewDecoder(r.Body).Decode(&priv)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ws, err := wallet.NewWallets(nodeId)
	if err != nil {
		panic(err)
	}
	pub, addr := ws.LoadPrivate(priv.Privatekey, nodeId)
	result := Wal{
		priv.Privatekey,
		addr,
		pub,
	}

	jsonData, err := json.Marshal(result)
	w.Header().Set("Content-Type", "application/json")

	_, err = w.Write(jsonData)
	if err != nil {
		log.Println(err)
	}
}

type Tras struct {
	Transactions []Transaction `json:"Transactions"`
}

//获取历史交易
func (bc *Blockchain) gethistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	result := Tras{
		Transactions: bc.GetTransationHashes(),
	}
	jsonData, err := json.Marshal(result)
	w.Header().Set("Content-Type", "application/json")

	_, err = w.Write(jsonData)
	if err != nil {
		log.Println(err)
	}
}

type Wal struct {
	Privatekey string
	Address    string
	Publickey  string
}

func Hello(w http.ResponseWriter, r *http.Request) {
	wallets, _ := wallet.NewWallets(r.URL.Port()) //从钱包文件读取所有的钱包
	address := wallets.CreateWallet()             //创建新钱包
	wallets.SaveToFile(r.URL.Port())              //创建完成后，保存到本地，不参与网络共享，必须自己保管好！

	private := wallets.Wallets[address].PrivateKey.D.Bytes()
	privateStr := hex.EncodeToString(private)
	publicStr := hex.EncodeToString(wallets.Wallets[address].PublicKey)

	result := Wal{
		privateStr,
		address,
		publicStr,
	}

	jsonData, err := json.Marshal(result)
	w.Header().Set("Content-Type", "application/json")

	_, err = w.Write(jsonData)
	if err != nil {
		log.Println(err)
	}
}
