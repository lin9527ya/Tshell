package command

import (
	"BackendTemplate/pkg/utils"
	"strings"
	"sync"
)

type FileBrowserQueue struct {
	mutex  sync.Mutex
	Queues map[string]chan string
}

var VarFileBrowserQueue = &FileBrowserQueue{Queues: make(map[string]chan string)}

func (q *FileBrowserQueue) Add(uid string, files string) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	if _, exists := q.Queues[uid]; !exists {
		q.Queues[uid] = make(chan string, 1)
	}
	select {
	case <-q.Queues[uid]: // 清空旧数据
	default: // 若通道为空，继续发送
	}

	// 发送最新的 pids 数据
	q.Queues[uid] <- files
}

func (q *FileBrowserQueue) GetOrCreateQueue(uid string) chan string {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	if _, exists := q.Queues[uid]; !exists {
		q.Queues[uid] = make(chan string, 1) // 带缓冲区的通道，防止阻塞
	}
	return q.Queues[uid]
}

type FileNode struct {
	Name         string      `json:"name"`
	Size         string      `json:"size"`
	Type         string      `json:"type"` // "D" 表示目录，"F" 表示文件
	Path         string      `json:"path"`
	ModifiedTime string      `json:"modifiedTime,omitempty"`
	Permissions  string      `json:"permissions,omitempty"`
	Children     []*FileNode `json:"children,omitempty"`
}

var UidFileBrowser = make(map[string][]*FileNode)

// fileBrowserMu 保护 UidFileBrowser 的并发读写（断线清理与文件浏览并发时避免 map 竞争 panic）
var fileBrowserMu sync.Mutex

// ClearUidFileBrowser 加锁删除指定客户端的文件浏览缓存（供 ExitClient/断线清理调用）
func ClearUidFileBrowser(uid string) {
	fileBrowserMu.Lock()
	defer fileBrowserMu.Unlock()
	delete(UidFileBrowser, uid)
}

func ParseDirectoryString(uid string, data string) []*FileNode {
	fileBrowserMu.Lock()
	defer fileBrowserMu.Unlock()

	// 空数据校验与预处理
	if strings.TrimSpace(data) == "" {
		return UidFileBrowser[uid]
	}
	
	lines := strings.Split(data, "\n")
	if len(lines) < 4 {
		return UidFileBrowser[uid]
	}
	
	// 优化路径处理
	line0 := strings.TrimSuffix(lines[0], "/*")
	line0 = strings.ReplaceAll(line0, "\\", "/") // 使用ReplaceAll更高效
	line0 = strings.TrimSuffix(line0, "/")
	
	var dirs []string
	var pan string
	
	if strings.HasPrefix(data, "/") { // linux
		pan = "/"
		if line0 != "/" {
			dirs = strings.Split(line0, "/")[1:]
		}
	} else { // windows
		if len(line0) >= 2 {
			pan = line0[:2]
			dirs = strings.Split(line0, "/")[1:]
		} else {
			return UidFileBrowser[uid]
		}
	}
	
	// 初始化文件浏览器结构
	if _, exists := UidFileBrowser[uid]; !exists {
		if strings.HasPrefix(data, "/") {
			UidFileBrowser[uid] = []*FileNode{{Name: "/", Type: "D", Path: "/"}}
		} else {
			UidFileBrowser[uid] = []*FileNode{{Name: pan, Type: "D", Path: pan}}
		}
	} else {
		if !exsitPan(UidFileBrowser[uid], pan) {
			UidFileBrowser[uid] = append(UidFileBrowser[uid], &FileNode{Name: pan, Type: "D", Path: pan})
		}
	}

	// 创建当前目录节点
	var tmpDirNode FileNode
	if len(dirs) > 0 {
		tmpDirNode = FileNode{Name: dirs[len(dirs)-1], Path: line0, Type: "D", Size: "0"}
	} else {
		if strings.HasPrefix(data, "/") {
			tmpDirNode = FileNode{Name: "/", Path: line0, Type: "D", Size: "0"}
		} else {
			tmpDirNode = FileNode{Name: pan, Path: line0, Type: "D", Size: "0"}
		}
	}
	
	// 优化文件解析 - 预分配切片容量
	fileCount := len(lines) - 3
	if fileCount > 0 {
		tmpDirNode.Children = make([]*FileNode, 0, fileCount)
	}
	
	// 快速文件解析
	for i := 3; i < len(lines); i++ {
		line := lines[i]
		if line == "" {
			continue
		}
		
		contents := strings.Split(line, "\t")
		if len(contents) < 4 {
			continue // 跳过格式错误的行
		}
		
		var tmpNode FileNode
		
		// 支持新格式：Type\tSize\tModTime\tPermissions\tName
		// 也兼容旧格式：Type\tSize\tModTime\tName
		if len(contents) >= 5 {
			// 新格式：包含权限信息
			fileName := contents[4]
			filePath := line0 + "/" + fileName
			
			if contents[0] == "F" {
				tmpNode = FileNode{
					Name:         fileName,
					Size:         utils.BytesToSize(contents[1]),
					Path:         filePath,
					Type:         contents[0],
					ModifiedTime: contents[2],
					Permissions:  contents[3],
				}
			} else {
				tmpNode = FileNode{
					Name:         fileName,
					Path:         filePath,
					Type:         contents[0],
					ModifiedTime: contents[2],
					Permissions:  contents[3],
				}
			}
		} else {
			// 旧格式：不包含权限信息
			fileName := contents[3]
			filePath := line0 + "/" + fileName
			
			if contents[0] == "F" {
				tmpNode = FileNode{
					Name:         fileName,
					Size:         utils.BytesToSize(contents[1]),
					Path:         filePath,
					Type:         contents[0],
					ModifiedTime: contents[2],
					Permissions:  "-",
				}
			} else {
				tmpNode = FileNode{
					Name:         fileName,
					Path:         filePath,
					Type:         contents[0],
					ModifiedTime: contents[2],
					Permissions:  "-",
				}
			}
		}
		
		tmpDirNode.Children = append(tmpDirNode.Children, &tmpNode)
	}

	//logx.Println("data:", data)
	//logx.Println("dirs:", dirs)
	//logx.Println("pan:", pan)
	//logx.Println("tmpDirNode:", &tmpDirNode)
	if len(dirs) > 0 {
		for i, u := range UidFileBrowser[uid] {
			if u.Name == pan {
				addToDirectoryTree(UidFileBrowser[uid][i], &tmpDirNode, dirs[:len(dirs)-1])
			}
		}
	} else {
		for i, u := range UidFileBrowser[uid] {
			if u.Name == pan {
				addToDirectoryTree(UidFileBrowser[uid][i], &tmpDirNode, dirs)
			}
		}
	}

	// 缓存树节点上限：长会话反复浏览大目录时防止无界增长（超限清空重建）
	if nodeCount(UidFileBrowser[uid]) > maxDirTreeNodes {
		UidFileBrowser[uid] = nil
		UidFileBrowser[uid] = []*FileNode{{Name: pan, Type: "D", Path: pan}}
	}
	return UidFileBrowser[uid]
}

// maxDirTreeNodes 单个客户端的文件树缓存节点上限
const maxDirTreeNodes = 20000

// nodeCount 统计文件树节点总数
func nodeCount(nodes []*FileNode) int {
	total := len(nodes)
	for _, n := range nodes {
		if n.Children != nil {
			total += nodeCount(n.Children)
		}
	}
	return total
}
func isInChild(root *FileNode, child *FileNode) (in bool) {
	in = false
	for _, childNode := range root.Children {
		if childNode.Name == child.Name {
			in = true
			break
		}
	}
	return in
}
func deleteChild(root []*FileNode, child *FileNode) (result []*FileNode) {
	for _, childNode := range root {
		if childNode.Name != child.Name {
			result = append(result, childNode)
		}
	}
	return result
}
func exsitPan(filenode []*FileNode, pan string) bool {
	for _, file := range filenode {
		if file.Name == pan {
			return true
		}
	}
	return false
}
func addToDirectoryTree(root *FileNode, node *FileNode, paths []string) {
	//logx.Println("path:", paths)
	current := root
	if len(paths) > 0 {
		for _, part := range paths {
			// 查找或创建当前层级的文件夹节点
			found := false
			for _, child := range current.Children {
				if child.Name == part && child.Type == "D" {
					current = child
					found = true
					break
				}
			}
			// 如果未找到该层级文件夹，则创建
			if !found {
				var path string
				if strings.HasSuffix(current.Path, "/") {
					path = strings.TrimSuffix(current.Path, "/")
				} else {
					path = current.Path
				}
				newDir := &FileNode{Name: part, Type: "D", Path: path + "/" + part}
				current.Children = append(current.Children, newDir)
				current = newDir
			}
		}
		found := false
		for _, child := range current.Children {
			if child.Name == node.Name && child.Type == "D" {
				for _, child2 := range node.Children {
					if !isInChild(child, child2) {
						child.Children = append(child.Children, child2)
					} else {
						for _, childNode := range child.Children {
							if childNode.Name == child2.Name {
								childNode.Size = child2.Size
								childNode.ModifiedTime = child2.ModifiedTime
								childNode.Permissions = child2.Permissions
							}
						}
					}

				}
				for _, child2 := range child.Children {
					if !isInChild(node, child2) {
						child.Children = deleteChild(child.Children, child2)
					}
				}
				found = true
				break
			}
		}
		if !found {
			current.Children = append(current.Children, node)
		}
	} else {
		//logx.Println("current.Name:", current.Name)
		//logx.Println("node.Name:", node.Name)
		if current.Name == node.Name {
			for _, child := range node.Children {
				if !isInChild(current, child) {
					current.Children = append(current.Children, child)
				} else {
					for _, childNode := range current.Children {
						if childNode.Name == child.Name {
							childNode.Size = child.Size
							childNode.ModifiedTime = child.ModifiedTime
							childNode.Permissions = child.Permissions
						}
					}
				}
			}
			for _, child := range current.Children {
				if !isInChild(node, child) {
					child.Children = deleteChild(child.Children, child)
				}
			}
		} else {
			found := false
			for _, child := range current.Children {
				if child.Name == node.Name && child.Type == "D" {
					for _, child2 := range node.Children {
						if !isInChild(child, child2) {
							child.Children = append(child.Children, child2)
						} else {
							for _, childNode := range child.Children {
								if childNode.Name == child2.Name {
									childNode.Size = child2.Size
									childNode.ModifiedTime = child2.ModifiedTime
									childNode.Permissions = child2.Permissions
								}
							}
						}
					}
					for _, child2 := range child.Children {
						if !isInChild(node, child2) {
							child.Children = deleteChild(child.Children, child2)
						}
					}
					found = true
					break
				}
			}
			if !found {
				current.Children = append(current.Children, node)
			}
		}
	}

}
// Remove 删除指定客户端的文件浏览通道（断线/删除客户端时调用）
func (q *FileBrowserQueue) Remove(uid string) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	delete(q.Queues, uid)
}
