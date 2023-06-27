package client

import (
	"fmt"
	"log"
	"strconv"
	"zzschain/core"
	"zzschain/wallet"
)

//createBlockchain 创建全新区块链
func (cli *CLI) createBlockchain(address string, nodeID string) {
	if !wallet.ValidateAddress(address) {
		log.Panic("ERROR: 地址非法")
	}
	bc := core.CreatBlockchain([]byte(address), nodeID) //注意，这里调用的是blockchain.go中的函数
	//bc := NewBlockchain()
	defer bc.Database.Close()

	UTXOSet := core.UTXOSet{bc}
	UTXOSet.Reindex() //在数据库中建立UTXO

	fmt.Println("创建全新区块链完毕！")
}

//createWallet 创建钱包并且保存到本地
func (cli *CLI) createWallet(nodeID string) {
	wallets, _ := wallet.NewWallets(nodeID) //从钱包文件读取所有的钱包
	address := wallets.CreateWallet()       //创建新钱包
	wallets.SaveToFile(nodeID)              //创建完成后，保存到本地，不参与网络共享，必须自己保管好！

	fmt.Printf("你的新钱包地址是: %s\n", address)
}

//GetBalance 获得账号余额
func (cli *CLI) getBalance(address string, nodeID string) {
	if !wallet.ValidateAddress(address) {
		log.Panic("ERROR: 地址非法")
	}
	bc := core.NewBlockchain(nodeID)
	defer bc.Database.Close()

	balance := 0
	pubKeyHash := wallet.Base58Decode([]byte(address))
	pubKeyHash = pubKeyHash[1 : len(pubKeyHash)-4]

	UTXOSet := core.UTXOSet{bc}
	UTXOs := UTXOSet.FindUTXO(pubKeyHash)

	for _, output := range UTXOs {
		balance += output.Value
	}

	fmt.Printf("'%s'的账号余额是: %x\n", address, balance)
}

//listAddresses 列出所有钱包的地址
func (cli *CLI) listAddresses(nodeID string) {
	wallets, err := wallet.NewWallets(nodeID)
	if err != nil {
		log.Panic(err)
	}
	addresses := wallets.GetAddresses()

	for _, address := range addresses {
		fmt.Println(address)
	}
}

// printChain 打印区块，从最新到最旧，直到打印完成创始区块
func (cli *CLI) printChain(nodeID string) {
	bc := core.NewBlockchain(nodeID)
	defer bc.Database.Close()
	bci := bc.Iterator()

	for {
		block := bci.Next()

		fmt.Printf("Prev. Hash:%x\n", block.PrevHash)
		//fmt.Printf("Data:%s\n", block.Data)
		fmt.Printf("Hash:%x\n", block.Hash)
		pow := core.NewProofOfWork(block)
		fmt.Printf("PoW:%s\n", strconv.FormatBool(pow.Validate()))
		fmt.Println()

		for _, tx := range block.Transactions {
			fmt.Println(tx)
		}

		if core.IsInitBlock(block.PrevHash.Bytes()) { //创始区块的PrevBlockHash为byte[]{}
			break
		}
	}
}

func (cli *CLI) reindexUTXO(nodeID string) {
	bc := core.NewBlockchain(nodeID)
	UTXOSet := core.UTXOSet{bc}
	UTXOSet.Reindex()

	count := UTXOSet.CountTransactions()
	fmt.Printf("重建索引完成! 总共有%d个交易在UTXO集合中。\n", count)
}

//send 转账
func (cli *CLI) send(from string, to string, amount int, nodeID string, mineNow bool) {
	if !wallet.ValidateAddress(from) {
		log.Panic("ERROR: 发送地址非法")
	}
	if !wallet.ValidateAddress(to) {
		log.Panic("ERROR: 接收地址非法")
	}
	bc := core.NewBlockchain(nodeID) //打开数据库，读取区块链并构建区块链实例
	UTXOSet := core.UTXOSet{bc}
	defer bc.Database.Close() //转账完毕，关闭数据库
	wallets, err := wallet.NewWallets(nodeID)
	if err != nil {
		log.Panic(err)
	}
	wallet := wallets.GetWallet(from)
	tx := core.NewUTXOTransaction(&wallet, []byte(to), amount, &UTXOSet)
	if mineNow { //当前是挖矿节点，有奖励
		cbTx := core.NewCoinbaseTX([]byte(from), "")
		txs := []*core.Transaction{cbTx, tx}

		newBlock := bc.MineBlock(txs, from)
		UTXOSet.Update(newBlock)
	} else { //非挖矿节点
		sendTx(knownNodes[0], tx) //发送给中心节点
	}

	fmt.Println("转账成功！")
}
