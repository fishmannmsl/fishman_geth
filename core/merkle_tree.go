package core

import "crypto/sha256"

// MerkleTree Merkle树结构
type MerkleTree struct {
	RootNode *MerkleNode
}

// MerkleNode Merkle树节点
type MerkleNode struct {
	Left  *MerkleNode
	Right *MerkleNode
	Data  []byte
}

// NewMerkleTree 从一序列字节数组数据创建一个Merkle树
func NewMerkleTree(data [][]byte) *MerkleTree {
	var nodes []MerkleNode

	if len(data)%2 != 0 { //如果数组数量为奇数，将最后一个数据重复加入进来，使其成为偶数
		data = append(data, data[len(data)-1])
	}

	//建立叶子节点数组
	for _, datum := range data {
		node := NewMerkleNode(nil, nil, datum) //创建节点，left和right目前为nil，中间计算用
		nodes = append(nodes, *node)
	}

	//以叶子节点为基础，建立merkle树
	for i := 0; i < len(data)/2; i++ { //第一层循环，次数是data的长度的一半
		var newLevel []MerkleNode

		for j := 0; j < len(nodes); j += 2 { //内存每循环一次，nodes的长度减半
			//获得left和right叶子，但right和left构建的root为nil，
			//这个root的值取决于root如何从left和right叶子节点计算而来
			node := NewMerkleNode(&nodes[j], &nodes[j+1], nil)
			newLevel = append(newLevel, *node)
		}

		nodes = newLevel //将新的节点数组拷贝给最终的nodes，生成新的nodes
	}

	mTree := MerkleTree{&nodes[0]} //最后生成的nodes只有一个节点：树的最顶层的根节点，以此节点的指针构建merkle tree

	return &mTree
}

// NewMerkleNode 哈希计算根节点的方法，创建一个新的Merkle树节点
//叶子节点的left和right参数为nil，data参数不为nil，直接生成data的哈希就是节点值（节点的Data）
//其它节点的data参数为nil，left和right参数不为nil，组合right和left并哈希后生成节点的Data
func NewMerkleNode(left, right *MerkleNode, data []byte) *MerkleNode {
	mNode := MerkleNode{}

	if left == nil && right == nil { //如果叶子节点均为nil
		hash := sha256.Sum256(data)
		mNode.Data = hash[:]
	} else { //叶子节点存在，忽略最后一个参数MerkleNode
		prevHashes := append(left.Data, right.Data...)
		hash := sha256.Sum256(prevHashes)
		mNode.Data = hash[:]
	}

	mNode.Left = left
	mNode.Right = right

	return &mNode
}
