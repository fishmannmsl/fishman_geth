package core

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math"
	"math/big"
	"time"
)

var (
	maxNonce = math.MaxInt64 //避免计数溢出，设定计数上限
)

// ProofOfWork POW结构体
//可以看出，每一个pow实例与具体的block相关
//但在确定好挖矿难度系数后，所有区块的pow的target是相同的，
//除非挖矿系数随着时间推移，挖矿难度系数不断增加
type ProofOfWork struct {
	block  *Block   //指向区块的指针
	target *big.Int //必要条件：哈希后的数据转为大整数后，小于target
}

// NewProofOfWork 初始化创建一个POW的函数，以block指针为参数（将修改该block）
//主要目的是确定target
func NewProofOfWork(b *Block) *ProofOfWork {
	temp := new(big.Int).Exp(Big2, Big256, nil) // 2**256
	target := new(big.Int).Div(temp, b.Difficulty)
	pow := &ProofOfWork{b, target} //初始化创建一个POW
	//log.Infof("Target: %x\n", target)
	return pow
}

// prepareData 准备进行哈希计算的数据，注意，
//进行哈希的数据除了block结构的数据外，还增加了挖矿难度系数targetBits
func (pow *ProofOfWork) prepareData(nonce *big.Int) []byte {
	data := bytes.Join(
		[][]byte{
			pow.block.PrevHash.Bytes(),
			IntToHex(nonce.Int64()),
			IntToHex(pow.block.Number.Int64()),
			IntToHex(int64(pow.block.Reward)),
			IntToHex(pow.block.Timestamp),
		},
		[]byte{},
	)
	return data
}

// Run POW挖矿核心算法实现，注意，这是一个方法，不是函数，
// 因为挖矿的完整描述是：挖出包含某个实际交易信息（或数据）的区块
// 挖矿是为交易上链提供服务，矿工拿到交易信息后进行挖矿，挖出的有效区块将包含交易信息
// 有可能挖不出符合条件的区块，所以将区块上链之前，需要对挖出的区块进行验证（验证是否符合条件）
func (pow *ProofOfWork) Run() (*big.Int, []byte) {
	var hashInt big.Int //存储哈希转成的大数字
	var hash Hash       //数组
	nonce := 0          //计数器，初始化为0

	fmt.Printf("正在挖出一个新区块...\n")
	begin := time.Now().UnixNano()
	for nonce = 0; nonce < maxNonce; nonce++ { //有可能挖不出符合条件的区块
		data := pow.prepareData(big.NewInt(int64(nonce)))
		hash = sha256.Sum256(data)

		//hash[:]为创建的一个切片，该切片包含hash数组的所有元素
		//这里不是直接传递hash数组，而是传递切片（相当于数组的引用），
		//是为节约内存开销，因为有可能要计算非常多的次数，如果每次都是拷贝数组（数组是值类型），
		//那么这个算法的内存开销可能很大
		hashInt.SetBytes(hash[:])
		if hashInt.Cmp(pow.target) == -1 {
			//hashInt<pow.target，则挖矿成功，返回区块和有效计数器，不必继续挖
			break
		}
	}
	end := time.Now().UnixNano()
	consumedTime := (end - begin) / 1e6
	difficulty := pow.block.Difficulty
	if consumedTime < 50 {
		difficulty.Add(difficulty, big.NewInt(1638))
	} else {
		difficulty.Sub(difficulty, big.NewInt(1638))
	}
	pow.block.Difficulty = difficulty
	fmt.Println("完成")
	return big.NewInt(int64(nonce)), hash[:] //返回切片而不是直接返回数组对象，可重复使用该数组内存。
}

// Validate 验证工作量证明POW
func (pow *ProofOfWork) Validate() bool {
	var hashInt big.Int

	data := pow.prepareData(pow.block.Nonce) //挖矿完成后，block的Nonce即已确定
	hash := sha256.Sum256(data)
	hashInt.SetBytes(hash[:])

	isValid := hashInt.Cmp(pow.target) == -1 //哈希转成的大数字小于目标值，则返回-1，isValid为true

	return isValid
}
