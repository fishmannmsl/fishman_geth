package core

import (
	"bytes"
	"encoding/gob"
	"github.com/boltdb/bolt"
	"log"
	"math/big"
	"time"
)

//Block 区块结构新版，增加了计数器nonce，主要目的是为了校验区块是否合法
//即挖出的区块是否满足工作量证明要求的条件
type Block struct {
	Nonce        *big.Int       `json:"Nonce"`
	Number       *big.Int       `json:"Number"`
	Difficulty   *big.Int       `json:"Difficulty"`
	Reward       int            `json:"Reward"`
	Timestamp    int64          `json:"Timestamp"`
	Coinbase     []byte         `json:"Coinbase"`
	Hash         Hash           `json:"Hash"`
	PrevHash     Hash           `json:"PrevHash"`
	Transactions []*Transaction `json:"Transactions"`
}

//NewBlock 创建普通区块
//一个block里面可以包含多个交易
func NewBlock(
	transactions []*Transaction,
	db *bolt.DB,
	coinbase []byte,
) *Block {
	block := &Block{
		Reward:       Reward,
		Timestamp:    time.Now().Unix(),
		Coinbase:     coinbase,
		Transactions: transactions,
	}
	//判断是否为创世块
	if db == nil {
		block.Number = Big0
		block.Difficulty = big.NewInt(21955) //2195456
		block.PrevHash = [32]byte{}
	} else {
		err := db.View(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte(BlocksBucket))
			hash := b.Get([]byte("last"))
			lastb := b.Get(hash)
			lastblock := DeserializeBlock(lastb)
			block.Difficulty = lastblock.Difficulty
			block.Number = lastblock.Number.Add(lastblock.Number, Big1)
			block.PrevHash = lastblock.Hash
			return nil
		})
		if err != nil {
			panic(err)
		}
	}
	//挖矿实质上是算出符合要求的哈希,同时在pow中进行难度调整
	pow := NewProofOfWork(block) //注意传递block指针作为参数
	nonce, hash := pow.Run()
	//设置block的计数器和哈希
	block.Nonce = nonce
	block.Hash = BytesToHash(hash)

	return block
}

// HashTransactions 计算交易组合的哈希值，最后得到的是Merkle tree的根节点
//获得每笔交易的哈希，将它们关联起来，然后获得一个连接后的组合哈希
//此方法只会被PoW使用
func (b *Block) HashTransactions() []byte {
	var transactions [][]byte

	for _, tx := range b.Transactions {
		transactions = append(transactions, tx.Serialize())
	}
	mTree := NewMerkleTree(transactions)

	return mTree.RootNode.Data //返回Merkle tree的根节点
}

//NewGenesisBlock 创建创始区块，包含创始交易。注意，创建创始区块也需要挖矿。
func NewGenesisBlock(coninbase *Transaction, coinbase []byte) *Block {
	return NewBlock([]*Transaction{coninbase}, nil, coinbase)
}

//Serialize Block序列化
//特别注意，block对象的任何不以大写字母开头命令的变量，其值都不会被序列化到[]byte中
func (b *Block) Serialize() []byte {
	var result bytes.Buffer //定义一个buffer存储序列化后的数据

	//初始化一个encoder，gob是标准库的一部分
	//encoder根据参数的类型来创建，这里将编码为字节数组
	encoder := gob.NewEncoder(&result)
	err := encoder.Encode(b) //编码
	if err != nil {
		log.Panic(err) //如果出错，将记录log后，Panic调用，立即终止当前函数的执行
	}

	return result.Bytes()
}

// DeserializeBlock 反序列化，注意返回的是Block的指针（引用）
func DeserializeBlock(d []byte) *Block {
	var block Block //一般都不会通过指针来创建一个struct。记住struct是一个值类型

	decoder := gob.NewDecoder(bytes.NewReader(d))
	err := decoder.Decode(&block)
	if err != nil {
		log.Panic(err)
	}

	return &block //返回block的引用
}
