package client

import (
	"bytes"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/big"
	"net"
	"net/http"
	"zzschain/core"
)

const protocol = "tcp"   //通信协议
const nodeVersion = 1    //节点版本
const commandLength = 12 //命令长度：12个字节

var nodeAddress string                      //当前节点地址
var miningAddress string                    //挖矿节点地址
var knownNodes = []string{"localhost:3000"} //初始化为中心节点
var blocksInTransit = [][]byte{}            //待下载的区块，用于跟踪下载区块
var mempool = make(map[string]core.Transaction)

// addr 服务器列表
type addr struct {
	AddrList []string
}

//block 返回getblock请求的回复消息
type block struct {
	AddrFrom string
	Block    []byte
}

//getblocks getblocks命令的消息结构
type getblocks struct {
	AddrFrom string
}

//getdata getdata命令的消息结构
type getdata struct {
	AddrFrom string
	Type     string //请求数据的类型：block或tx
	ID       []byte //交易或块的ID
}

// inv 向其他节点展示当前节点有什么块或交易的消息结构，属于回复消息
//回复的消息内容与请求消息的类型有关，如果请求消息类型是tx，回复的消息内容就是交易，
//但如果请求的消息类型是block，回复的消息内容就是区块
type inv struct {
	AddrFrom string
	Type     string //对方请求的类型：tx或者block
	Items    [][]byte
}

//tx 交易信息请求消息结构
type tx struct {
	AddFrom     string
	Transaction []byte
}

// verzion 版本号请求消息结构
type verzion struct {
	Version    int      //本地区块链版本号
	BestNumber *big.Int //本地区块链中节点的高度
	AddrFrom   string   //发送此命令者的地址
}

//commandToBytes 将命令字符串转为byte字节
//直接将字符串中的每一个字符强制转换为byte类型
func commandToBytes(command string) []byte {
	var bytes [commandLength]byte //command占用12个字节

	for i, c := range command {
		bytes[i] = byte(c)
	}

	return bytes[:]
}

//bytesToCommand 将字节数组转为命令字符串，注意字符串最后一个字符是0x0
//每一个字节直接转为字符
func bytesToCommand(bytes []byte) string {
	var command []byte

	for _, b := range bytes {
		if b != 0x0 {
			command = append(command, b)
		}
	}

	return fmt.Sprintf("%s", command)
}

func extractCommand(request []byte) []byte {
	return request[:commandLength]
}

//requestBlocks 请求区块结构，本案例未用到
func requestBlocks() {
	for _, node := range knownNodes { //向多个节点发送区块请求消息
		sendGetBlocks(node)
	}
}

//sendAddr 发送可用服务节点信息
//这个函数在本案例中没有用到
func sendAddr(address string) {
	nodes := addr{knownNodes}
	nodes.AddrList = append(nodes.AddrList, nodeAddress)
	payload := gobEncode(nodes)
	request := append(commandToBytes("addr"), payload...) //命令：addr

	sendData(address, request)
}

//sendBlock 发送区块
func sendBlock(addr string, b *core.Block) {
	data := block{nodeAddress, b.Serialize()}
	payload := gobEncode(data)
	request := append(commandToBytes("block"), payload...) //命令block

	for i := 0; i < 2; i++ {
		sendData(addr, request)
	}
}

//sendData 通过网络将消息发送出去
func sendData(addr string, data []byte) {
	conn, err := net.Dial(protocol, addr) //连接到服务器

	if err != nil {
		fmt.Printf("%s is not available\n", addr)
		var updatedNodes []string

		for _, node := range knownNodes {
			if node != addr {
				updatedNodes = append(updatedNodes, node) //将本地节点添加到已知节点列表，公开
			}
		}

		knownNodes = updatedNodes

		return
	}
	defer conn.Close()
	_, err = io.Copy(conn, bytes.NewReader(data)) //将消息读取后写入给conn
	if err != nil {
		log.Panic(err)
	}
}

//sendInv 发送Inv请求：告诉我你有什么区块或者交易
func sendInv(address, kind string, items [][]byte) {
	inventory := inv{nodeAddress, kind, items} //kind为消息类型
	payload := gobEncode(inventory)
	request := append(commandToBytes("inv"), payload...) //命令inv

	for i := 0; i < 2; i++ {
		sendData(address, request)
	}
}

//sendGetBlocks 发送getblocks请求
func sendGetBlocks(address string) {
	payload := gobEncode(getblocks{nodeAddress})

	request := append(commandToBytes("getblocks"), payload...) //命令：getblocks

	for i := 0; i < 2; i++ {
		sendData(address, request)
	}
}

//sendGetData 发送数据请求
func sendGetData(address, kind string, id []byte) {
	payload := gobEncode(getdata{nodeAddress, kind, id})     //kind为数据类型：block/tx
	request := append(commandToBytes("getdata"), payload...) //命令：getdata

	for i := 0; i < 2; i++ {
		sendData(address, request)
	}
}

//sendTx 发送交易信息，这个不是服务器内部调用，而是由交易发起者从外部调用
//这是服务器上唯一由非矿工节点从外部调用的函数，功能是发起一个交易
func sendTx(addr string, tnx *core.Transaction) {
	data := tx{nodeAddress, tnx.Serialize()}
	payload := gobEncode(data)
	request := append(commandToBytes("tx"), payload...) //命令：tx

	for i := 0; i < 2; i++ {
		sendData(addr, request)
	}
}

//sendVersion 发送本地区块链版本信息
func sendVersion(addr string, bc *core.Blockchain) {
	bestNumber := bc.GetBestNumber()
	payload := gobEncode(verzion{nodeVersion, bestNumber, nodeAddress})

	request := append(commandToBytes("version"), payload...)
	for i := 0; i < 2; i++ {
		sendData(addr, request)
	}
}

//handleAddr：处理addr命令回复，本案例中未用到
func handleAddr(request []byte) {
	var buff bytes.Buffer
	var payload addr

	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	knownNodes = append(knownNodes, payload.AddrList...)
	fmt.Printf("There are %d known nodes now!\n", len(knownNodes))
	requestBlocks()
}

//handleBlock 处理block命令回复
func handleBlock(request []byte, bc *core.Blockchain) {
	var buff bytes.Buffer
	var payload block

	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	blockData := payload.Block
	block := core.DeserializeBlock(blockData)

	fmt.Println("接收到一个新区块!")
	bc.AddBlock(block)

	fmt.Printf("添加到区块： %x\n", block.Hash)

	if len(blocksInTransit) > 0 { //如果还有待下载的区块，继续请求下载，每次只请求一个
		blockHash := blocksInTransit[0]
		sendGetData(payload.AddrFrom, "block", blockHash)

		//执行UTXO集更新
		UTXOSet := core.UTXOSet{bc}
		UTXOSet.Update(block)
		if len(blocksInTransit) > 0 {
			//待下载区块更新，删除原来的第0个
			blocksInTransit = blocksInTransit[1:]
		}
	}
}

//handleInv 处理inv命令回复，执行sendGetdata命令
//无论请求的是多少数量的block或者tx，handleInv执行只请求一个block或者一个tx
func handleInv(request []byte, bc *core.Blockchain) {
	var buff bytes.Buffer
	var payload inv

	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	fmt.Printf("Recevied inventory with %d %s\n", len(payload.Items), payload.Type)

	if payload.Type == "block" {
		blocksInTransit = payload.Items //从Inv获得的正是对方发来的全部block哈希列表

		blockHash := payload.Items[0]                     //没有判断区块是否在本地已经存在，而是放在addblock中进行处理
		sendGetData(payload.AddrFrom, "block", blockHash) //向对方发送getdata命令，请求缺失的一个区块

		newInTransit := [][]byte{}
		for _, b := range blocksInTransit {
			if bytes.Compare(b, blockHash) != 0 { //删除即将请求的区块
				newInTransit = append(newInTransit, b) //更新本地的区块信息
			}
		}
		blocksInTransit = newInTransit
	}

	if payload.Type == "tx" {
		txID := payload.Items[0] //本案例中，不会存在传送多个tx的情形

		if mempool[hex.EncodeToString(txID)].ID == nil {
			sendGetData(payload.AddrFrom, "tx", txID) //向对方请求某条交易信息
		}
	}
}

//handleGetBlocks 处理getblocks命令，发送Inv命令
//将本地block哈希列表发给远程节点
func handleGetBlocks(request []byte, bc *core.Blockchain) {
	var buff bytes.Buffer
	var payload getblocks

	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	blocks := bc.GetBlockHashes()
	sendInv(payload.AddrFrom, "block", blocks) //将本地所有区块的哈希数组发给对方
}

//handleGetData 处理getdata命令，发送所需的某个具体block或者tx
func handleGetData(request []byte, bc *core.Blockchain) {
	var buff bytes.Buffer
	var payload getdata

	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	if payload.Type == "block" {
		block, err := bc.GetBlock([]byte(payload.ID))
		if err != nil {
			return
		}

		sendBlock(payload.AddrFrom, &block)
	}

	if payload.Type == "tx" {
		txID := hex.EncodeToString(payload.ID)
		tx := mempool[txID]

		sendTx(payload.AddrFrom, &tx)
		// delete(mempool, txID)
	}
}

//handleTx 矿工处理请求tx的回复消息
func handleTx(request []byte, bc *core.Blockchain) {
	var buff bytes.Buffer
	var payload tx

	buff.Write(request[commandLength:]) //request消息，前面12个字节是命令，后面是payload
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	txData := payload.Transaction
	tx := core.DeserializeTransaction(txData)
	mempool[hex.EncodeToString(tx.ID)] = tx //将交易丢到待上链的交易池中

	if nodeAddress == knownNodes[0] { //当前节点为中心节点，中心节点收到新交易
		for _, node := range knownNodes {
			if node != nodeAddress && node != payload.AddFrom {
				//将当前交易ID通过inv命令发送给既非当前节点也非交易发起者节点之外的所有其它节点
				sendInv(node, "tx", [][]byte{tx.ID})
			}
		}
	} else { //当前节点为非中心节点
		if len(mempool) >= 2 && len(miningAddress) > 0 { //如果当前是挖矿节点，打包发来的交易进行挖矿处理：minerAddress不为空值
		MineTransactions:
			var txs []*core.Transaction

			for id := range mempool {
				tx := mempool[id]
				fmt.Printf("%s to be veryfied...\n", hex.EncodeToString(tx.ID))

				if bc.VerifyTransaction(&tx) == true {
					txs = append(txs, &tx)
					fmt.Printf("%s veryfied true.\n", hex.EncodeToString(tx.ID))
				} else {
					fmt.Printf("%s veryfied false.\n", hex.EncodeToString(tx.ID))
				}
			}

			if len(txs) == 0 {
				fmt.Println("所有的新交易均非法! 等待新的交易...")
				return
			}

			cbTx := core.NewCoinbaseTX([]byte(miningAddress), "")
			txs = append(txs, cbTx)

			newBlock := bc.MineBlock(txs, miningAddress)
			UTXOSet := core.UTXOSet{bc}
			UTXOSet.Update(newBlock)

			fmt.Println("新区块已挖出!")

			for _, tx := range txs {
				txID := hex.EncodeToString(tx.ID)
				delete(mempool, txID) //从交易池中删除当前已经上链的全部交易
			}

			for _, node := range knownNodes {
				if node != nodeAddress {
					//将新的模块哈希通过inv命令发送给除本地节点之外的其他节点，通知对方进行本地区块链更新
					sendInv(node, "block", [][]byte{newBlock.Hash.Bytes()})
				}
			}

			if len(mempool) > 0 {
				goto MineTransactions
			}
		}
	}
}

// handleVersion 处理版本请求回复消息
func handleVersion(request []byte, bc *core.Blockchain) {
	fmt.Printf("handleVersion...\n")
	var buff bytes.Buffer
	var payload verzion

	buff.Write(request[commandLength:]) //request消息，前面12个字节（commandLength）为命令，后面是payload
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	myBestNumber := bc.GetBestNumber()
	fmt.Printf("myBestHeight is %d\n", myBestNumber)

	foreignerBestNumber := payload.BestNumber
	fmt.Printf("foreignerBestHeight is %d\n", foreignerBestNumber)

	if myBestNumber.Cmp(foreignerBestNumber) < 0 { //如果本地区块height小，请求缺失区块
		sendGetBlocks(payload.AddrFrom)
	} else if myBestNumber.Cmp(foreignerBestNumber) > 0 { //如果本地区块height大，发送本地最新版本信息给到对方，对方可以据此更新
		sendVersion(payload.AddrFrom, bc)
	}

	// sendAddr(payload.AddrFrom)
	if !nodeIsKnown(payload.AddrFrom) {
		knownNodes = append(knownNodes, payload.AddrFrom)
	}
}

//handleConnection 处理中心，根据命令执行命令处理函数
func handleConnection(conn net.Conn, bc *core.Blockchain) {
	request, err := ioutil.ReadAll(conn)
	if err != nil {
		log.Panic(err)
	}
	command := bytesToCommand(request[:commandLength])
	fmt.Printf("Received %s command\n", command)

	switch command {
	case "addr": //请求可用的节点，暂时没有用到
		handleAddr(request)
	case "block":
		handleBlock(request, bc)
	case "inv": //向其他节点展示当前节点有什么块或交易
		handleInv(request, bc)
	case "getblocks": //给我看看你有什么区块
		handleGetBlocks(request, bc)
	case "getdata":
		handleGetData(request, bc)
	case "tx":
		handleTx(request, bc)
	case "version":
		handleVersion(request, bc)
	default:
		fmt.Println("Unknown command!")
	}

	conn.Close()
}

// StartServer 启动一个节点
//minerAddress若是空值，为非挖矿节点，不为空值，为挖矿节点
func StartServer(nodeID string, minerAddress string) {
	nodeAddress = fmt.Sprintf("localhost:%s", nodeID)
	//如果当前是挖矿节点，那么miningAddress的长度不会为空，否则miningAddress是空值
	miningAddress = minerAddress

	ln, err := net.Listen(protocol, nodeAddress) //在节点监听连接
	if err != nil {
		log.Panic(err)
	}

	bc := core.NewBlockchain(nodeID)

	if nodeAddress != knownNodes[0] { //如果不是中心节点，发送Version命令，从网络（中心节点）请求缺失区块
		sendVersion(knownNodes[0], bc) //服务器启动后，非中心节点要干的第一件事，就是下载缺失区块
	}
	mux := bc.Rount()
	// 创建 HTTP 服务器
	server := &http.Server{
		Handler: mux,
	}
	// 在 Goroutine 中启用 HTTP 服务监听
	go func() {
		err := server.Serve(ln)
		if err != nil {
			fmt.Println("Failed to start HTTP server:", err)
			return
		}
	}()

	for {
		conn, err := ln.Accept() //阻塞，等待客户端连接
		if err != nil {
			log.Panic(err)
		}

		//并发模式，接收来自客户端的连接请求,对每一个到来的客户端连接创建一个处理连接的并发任务
		//一旦有连接，前面的阻塞解除，程序将执行到下面的协程
		go handleConnection(conn, bc)
	}
}

func gobEncode(data interface{}) []byte {
	var buff bytes.Buffer

	enc := gob.NewEncoder(&buff)
	err := enc.Encode(data)
	if err != nil {
		log.Panic(err)
	}

	return buff.Bytes()
}

// nodeIsKnown 节点地址是否在遗址节点列表中
func nodeIsKnown(addr string) bool {
	for _, node := range knownNodes {
		if node == addr {
			return true
		}
	}

	return false
}
