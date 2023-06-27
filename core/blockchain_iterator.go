package core

import (
	"log"

	"github.com/boltdb/bolt"
)

//BlockchainIterator 区块链迭代器，用于对区块链中的区块进行迭代
type BlockchainIterator struct {
	currentHash []byte
	db          *bolt.DB
}

//Iterator 每当需要对链中的区块进行迭代时候，我们就通过Blockchain创建迭代器
//注意，迭代器初始状态为链中的tip，因此迭代是从最新到最旧的进行获取
func (bc *Blockchain) Iterator() *BlockchainIterator {
	bci := &BlockchainIterator{bc.Tip, bc.Database}
	return bci
}

//Next 区块链迭代，返回当前区块，并更新迭代器的currentHash为当前区块的PrevBlockHash
func (i *BlockchainIterator) Next() *Block {
	var block *Block

	err := i.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(BlocksBucket))
		encodeBlock := b.Get(i.currentHash)
		block = DeserializeBlock(encodeBlock)

		return nil
	})

	if err != nil {
		log.Panic(err)
	}

	i.currentHash = block.PrevHash.Bytes()

	return block
}
