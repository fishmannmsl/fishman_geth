package main

import (
	"zzschain/client"
	"zzschain/core"
)

func main() {
	//生成创世块所在文件，以便其它节点启动时能进行同步
	core.Init_BlockChain()
	cli := client.CLI{}
	cli.Run()
}
