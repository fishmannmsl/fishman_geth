package core

import (
	"encoding/hex"
	"fmt"
	"log"

	"github.com/boltdb/bolt"
)

//存储UTXO，目的是优化FindUTXO，不用迭代整个区块链（也就不用下载完整区块链）
const utxoBucket = "chainstate"

//存储UTXOBLOCK的哈希，目的是优化FindTransaction
const utxoBlockBucket = "chainstate_blockid2tx"

// UTXOSet 代表UTXO集合
type UTXOSet struct {
	Blockchain *Blockchain
}

// FindSpendableOutputs 从数据库的UTXO表中找到输入引用的未花费输出
//从未花费交易里取出未花费的输出，直至取出输出的币总数大于或等于需要send的币数为止
func (u UTXOSet) FindSpendableOutputs(pubkeyHash []byte, amount int) (int, map[string][]int) {
	unspentOutputs := make(map[string][]int)
	accumulated := 0 //sender发出的转出的全部币数
	db := u.Blockchain.Database

	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(utxoBucket))
		c := b.Cursor()

	Work:
		for k, v := c.First(); k != nil; k, v = c.Next() {
			txID := hex.EncodeToString(k)
			outs := DeserializeOutputs(v)

			for outIdx, out := range outs.Outputs { //得到足够的未花费输出（不少于需要转账的金额）
				if out.IsLockedWithKey(pubkeyHash) && accumulated < amount {
					accumulated += out.Value
					unspentOutputs[txID] = append(unspentOutputs[txID], outIdx)
				}
				if accumulated >= amount {
					break Work //退出两个循环
				}
			}
		}

		return nil
	})
	if err != nil {
		log.Panic(err)
	}

	return accumulated, unspentOutputs
}

// FindUTXO 从数据库的UTXO表中查找一个公钥哈希的UTXO
func (u UTXOSet) FindUTXO(pubKeyHash []byte) []TxOutput {
	var UTXOs []TxOutput
	db := u.Blockchain.Database

	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(utxoBucket))
		c := b.Cursor()

		for k, v := c.First(); k != nil; k, v = c.Next() {
			outs := DeserializeOutputs(v)

			for _, out := range outs.Outputs {
				strpubKeyHash := hex.EncodeToString(out.PubKeyHash)
				fmt.Println(strpubKeyHash)
				if out.IsLockedWithKey(pubKeyHash) {
					UTXOs = append(UTXOs, out)
				}
			}
		}

		return nil
	})
	if err != nil {
		log.Panic(err)
	}

	return UTXOs
}

// CountTransactions 从数据库的UTXO表中查找一个UTXO集合中交易的数量
func (u UTXOSet) CountTransactions() int {
	db := u.Blockchain.Database
	counter := 0

	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(utxoBucket))
		c := b.Cursor()

		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			counter++
		}

		return nil
	})
	if err != nil {
		log.Panic(err)
	}

	return counter
}

// Reindex 重建数据库的UTXO
//只会在区块链新创建完毕后执行一次，其他时候不执行
//在bucket中，一个交易ID，最多只有一条记录
//创建两个表：utxoBucket和utxoBlockBucket
func (u UTXOSet) Reindex() {
	db := u.Blockchain.Database
	bucketName := []byte(utxoBucket)
	bucketBlockName := []byte(utxoBlockBucket)

	err := db.Update(func(tx *bolt.Tx) error {
		err := tx.DeleteBucket(bucketName)               //如果bucket已经存在，删除它
		if err != nil && err != bolt.ErrBucketNotFound { //bucket已经存在但删除失败，返回
			log.Panic(err)
		}

		_, err = tx.CreateBucket(bucketName) //创建新的bucket
		if err != nil {
			log.Panic(err)
		}

		err = tx.DeleteBucket(bucketBlockName)           //如果bucket已经存在，删除它
		if err != nil && err != bolt.ErrBucketNotFound { //bucket已经存在但删除失败，返回
			log.Panic(err)
		}

		_, err = tx.CreateBucket(bucketBlockName) //创建新的bucket
		if err != nil {
			log.Panic(err)
		}

		return nil
	})
	if err != nil {
		log.Panic(err)
	}

	UTXO, UTXOBlock := u.Blockchain.FindUTXO() //获得所有的UTXOBlock

	err = db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketName)
		bd := tx.Bucket(bucketBlockName)
		for txID, outs := range UTXO {
			key, err := hex.DecodeString(txID)
			if err != nil {
				log.Panic(err)
			}

			//key为交易ID，value是该交易ID的所有未花费输出
			//所以，在bucket中，一个交易ID，最多只有一条记录（如果该交易没有未花费支持，那么不会存在该记录ID对应的记录）
			err = b.Put(key, outs.Serialize())
			if err != nil {
				log.Panic(err)
			}

			//更新或插入UTXOBlock，如果key相同，自动覆盖
			err = bd.Put(key, UTXOBlock[txID].Bytes())
			if err != nil {
				log.Panic(err)
			}
		}

		return nil
	})
}

// Update 根据区块中的交易更新数据库的UTXO表和UTXOBlock表
// 该区块是区块链的Tip区块
//需要处理的问题是：由于奖励固定，对于同一挖矿人，coinbasetx的ID相同，因此更新时候，
//如果交易中包含coinbase，那么只能删除一个，不能把UTXO中的该ID对应的coinbase全部删了
func (u UTXOSet) Update(block *Block) {
	db := u.Blockchain.Database

	err := db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(utxoBucket))
		blockb, err := tx.CreateBucketIfNotExists([]byte(utxoBlockBucket))
		if err != nil {
			log.Panic(err)
		}
		for _, tx := range block.Transactions {
			if tx.IsCoinbase() == false { //coninbase交易不含实质的输入，也就不对该交易的输入进行处理
				for _, vin := range tx.Vin {
					updatedOuts := TxOutputs{}
					outsBytes := b.Get(vin.Txid)
					if outsBytes != nil {
						outs := DeserializeOutputs(outsBytes)

						for outIdx, out := range outs.Outputs {
							if outIdx != vin.Vout { //如果UTXO中的输出不包含在当前交易中，保留到更新的UTXO集中
								updatedOuts.Outputs = append(updatedOuts.Outputs, out)
							}
						}

						if len(updatedOuts.Outputs) == 0 { //如果更新的UTXO的元素个数为0，从UTXO集中删除它
							err := b.Delete(vin.Txid)
							if err != nil {
								log.Panic(err)
							}
						} else { //如果更新的UTXO的元素个数不为0，更新UTXO
							err := b.Put(vin.Txid, updatedOuts.Serialize())
							if err != nil {
								log.Panic(err)
							}
						}
					}

				}
			}

			//将新交易的输出加入到UTXO中
			newOutputs := TxOutputs{}
			for _, out := range tx.Vout {
				newOutputs.Outputs = append(newOutputs.Outputs, out)
			}

			err := b.Put(tx.ID, newOutputs.Serialize())
			if err != nil {
				log.Panic(err)
			}

			//更新UTXOBlock
			err = blockb.Put(tx.ID, block.Hash.Bytes())
			if err != nil {
				log.Panic(err)
			}
		}

		return nil
	})
	if err != nil {
		log.Panic(err)
	}
}
