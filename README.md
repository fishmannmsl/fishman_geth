#### 项目介绍

在参考`go-ethereum`源码与`metamask`后用制作出的一个区块链钱包项目，具有 `创链`、`生成钱包`、`挖矿`、`转账`等功能

#### 项目运行

##### 初始化环境

```shell
#进入项目根目录后打开终端
$ go mod tidy
```

##### 运行

```shell
#终端输入,不加参数默认使用 3000端口，并且产生交易才开始挖矿，本节点为矿工（接受奖励的地址在初始链时自动生成）
$ go run main.go startnode
#startnode -port NodeId -miner Address - 通过特定的环境变量NODE_ID启动一个节点，可选参数：-miner启动挖矿 
```

#### 其它信息

- 共识机制
  - pow：非前导零进行难度计算
- 获取余额
  - UTXO
  - Merkle
- 节点同步
  - tcp：使用`tcp`请求进行节点之间的同步与通信
- 页面交互
  - http：使用`携程(go)`启动`web`服务器，用通道`chanl`进行`web`服务与`tcp`请求之间的数据交互
- 持久化
  - bolt：使用`bolt`数据库实现区块链的持久化