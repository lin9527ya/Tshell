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
	Children     []*FileNode `json:"children,omitempty"`
}

var UidFileBrowser = make(map[string][]*FileNode)

func ParseDirectoryString(uid string, data string) []*FileNode {
	var dirs []string
	var pan string
	lines := strings.Split(data, "\n")

	line0 := strings.TrimSuffix(lines[0], "/*")
	line0 = strings.Replace(line0, "\\", "/", -1)
	line0 = strings.TrimSuffix(line0, "/")
	if strings.HasPrefix(data, "/") { //linux
		pan = "/"
		dirs = strings.Split(line0, "/")[1:]
	} else { // windows
		pan = line0[:2]
		dirs = strings.Split(line0, "/")
		dirs = dirs[1:]
	}

	if _, exists := UidFileBrowser[uid]; !exists {
		if strings.HasPrefix(data, "/") { //linux
			UidFileBrowser[uid] = []*FileNode{{Name: "/", Type: "D", Path: "/"}}
		} else { // windows
			UidFileBrowser[uid] = []*FileNode{{Name: data[:2], Type: "D", Path: data[:2]}}
		}
	} else {
		if !exsitPan(UidFileBrowser[uid], pan) {
			UidFileBrowser[uid] = append(UidFileBrowser[uid], &FileNode{Name: data[:2], Type: "D", Path: data[:2]})
		}
	}

	var tmpDirNode FileNode
	if len(dirs) > 0 {
		tmpDirNode = FileNode{Name: dirs[len(dirs)-1], Path: line0, Type: "D", Size: "0"}
	} else {
		if strings.HasPrefix(data, "/") { //linux
			tmpDirNode = FileNode{Name: "/", Path: line0, Type: "D", Size: "0"}
		} else { // windows
			tmpDirNode = FileNode{Name: data[:2], Path: line0, Type: "D", Size: "0"}
		}

	}

	for _, line := range lines[3:] {
		contents := strings.Split(line, "\t")
		var tmpNode FileNode
		if contents[0] == "F" {
			tmpNode = FileNode{
				Name:         contents[3],
				Size:         utils.BytesToSize(contents[1]),
				Path:         line0 + "/" + contents[3],
				Type:         contents[0],
				ModifiedTime: contents[2],
			}
		} else {
			tmpNode = FileNode{
				Name:         contents[3],
				Path:         line0 + "/" + contents[3],
				Type:         contents[0],
				ModifiedTime: contents[2],
			}
		}

		tmpDirNode.Children = append(tmpDirNode.Children, &tmpNode)
	}

	//fmt.Println("data:", data)
	//fmt.Println("dirs:", dirs)
	//fmt.Println("pan:", pan)
	//fmt.Println("tmpDirNode:", &tmpDirNode)
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
	return UidFileBrowser[uid]
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
	//fmt.Println("path:", paths)
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
		//fmt.Println("current.Name:", current.Name)
		//fmt.Println("node.Name:", node.Name)
		if current.Name == node.Name {
			for _, child := range node.Children {
				if !isInChild(current, child) {
					current.Children = append(current.Children, child)
				} else {
					for _, childNode := range current.Children {
						if childNode.Name == child.Name {
							childNode.Size = child.Size
							childNode.ModifiedTime = child.ModifiedTime
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
