package client

import (
	"flag"
	"fmt"
	"log"
	"os"
	"zzschain/wallet"
)

// CLI 响应处理命令行参数
type CLI struct {
	NodeId  string
	Address string
}

// printUsage 打印命令行帮助信息
func (cli *CLI) printUsage() {
	fmt.Println("Usage:")
	fmt.Println("   send -from FROM -to TO -amount AMOUNT -mine - 发送amount数量的币，从地址FROM到TO,如果设定了-mine，则由本节点完成挖矿")
	fmt.Println("   startnode -port NodeId -miner Address - 通过特定的环境变量NODE_ID启动一个节点，可选参数：-miner启动挖矿")
}

// validateArgs 校验命令，如果无效，打印使用说明
func (cli *CLI) validateArgs() {
	if len(os.Args) < 2 { //所有命令至少有两个参数，第一个是程序名称，第二个是命名名称
		cli.printUsage()
		os.Exit(1)
	}
}

// Run 读取命令行参数，执行相应的命令
// 使用标准库里面的 flag 包来解析命令行参数：
func (cli *CLI) Run() {
	cli.validateArgs()

	//定义名称为"sendCmd"的空的flagset集合
	sendCmd := flag.NewFlagSet("send", flag.ExitOnError)
	startNodeCmd := flag.NewFlagSet("startnode", flag.ExitOnError)

	sendFrom := sendCmd.String("from", "", "钱包源地址")
	sendTo := sendCmd.String("to", "", "钱包目的地址")
	sendAmount := sendCmd.Int("amount", 0, "转移资金的数量")
	sendMine := sendCmd.Bool("mine", false, "在该节点立即挖矿")
	startNodePort := startNodeCmd.String("port", "", "启动节点，并制定节点的端口")
	startNodeMiner := startNodeCmd.String("miner", "", "启动挖矿模式，并制定奖励的钱包ADDRESS")

	//os.Args包含以程序名称开始的命令行参数
	switch os.Args[1] { //os.Args[0]为程序名称，真正传递的参数index从1开始，一般而言Args[1]为命令名称
	case "send":
		err := sendCmd.Parse(os.Args[2:])
		if err != nil {
			log.Panic(err)
		}
	case "startnode":
		err := startNodeCmd.Parse(os.Args[2:])
		if err != nil {
			log.Panic(err)
		}
	default:
		cli.printUsage()
		os.Exit(1)
	}

	if sendCmd.Parsed() {
		if *sendFrom == "" || *sendTo == "" || *sendAmount <= 0 {
			sendCmd.Usage()
			os.Exit(1)
		}

		cli.send(*sendFrom, *sendTo, *sendAmount, cli.NodeId, *sendMine)
	}

	if startNodeCmd.Parsed() {
		if *startNodePort == "" {
			startNodeCmd.Usage()
			os.Exit(1)
		}
		cli.startNode(*startNodePort, *startNodeMiner)
	}
}

func (cli *CLI) startNode(nodeID string, minerAddress string) {
	fmt.Printf("开始节点 %s\n", nodeID)
	if len(minerAddress) > 0 {
		if wallet.ValidateAddress(minerAddress) {
			fmt.Println("挖矿正在进行中. 接收挖矿奖励的地址: ", minerAddress)
		} else {
			log.Panic("错误的挖矿地址!")
		}
	}
	StartServer(nodeID, minerAddress) //启动节点服务器：区块链中每一个节点都是服务器
}
