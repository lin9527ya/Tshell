#!/usr/bin/env node

/**
 * 将前端构建产物复制到后端的脚本
 * 用于 Vue 3 + Vite 前端与 Go + Gin 后端的集成
 */

const fs = require('fs');
const path = require('path');
const { execSync } = require('child_process');

// 配置路径
const FRONTEND_DIST = path.resolve(__dirname, '../dist');
const BACKEND_EMBED_DIR = path.resolve(__dirname, '../../Tshell-server/dist');
const BACKEND_MAIN_FILE = path.resolve(__dirname, '../../Tshell-server/main.go');

// 颜色输出
const colors = {
  red: '\x1b[31m',
  green: '\x1b[32m',
  yellow: '\x1b[33m',
  blue: '\x1b[34m',
  reset: '\x1b[0m'
};

function log(level, message) {
  const timestamp = new Date().toLocaleTimeString();
  const colorMap = {
    info: colors.blue,
    success: colors.green,
    warning: colors.yellow,
    error: colors.red
  };
  
  console.log(`${colorMap[level]}[${level.toUpperCase()}]${colors.reset} ${timestamp} - ${message}`);
}

// 递归复制目录
function copyDirectory(src, dest) {
  try {
    // 确保目标目录存在
    if (!fs.existsSync(dest)) {
      fs.mkdirSync(dest, { recursive: true });
    }
    
    const entries = fs.readdirSync(src, { withFileTypes: true });
    
    for (const entry of entries) {
      const srcPath = path.join(src, entry.name);
      const destPath = path.join(dest, entry.name);
      
      if (entry.isDirectory()) {
        copyDirectory(srcPath, destPath);
      } else {
        fs.copyFileSync(srcPath, destPath);
      }
    }
  } catch (error) {
    throw new Error(`复制目录失败: ${error.message}`);
  }
}

// 获取目录大小
function getDirectorySize(dirPath) {
  let totalSize = 0;
  
  function traverse(currentPath) {
    const items = fs.readdirSync(currentPath);
    
    for (const item of items) {
      const itemPath = path.join(currentPath, item);
      const stats = fs.statSync(itemPath);
      
      if (stats.isDirectory()) {
        traverse(itemPath);
      } else {
        totalSize += stats.size;
      }
    }
  }
  
  try {
    traverse(dirPath);
    return totalSize;
  } catch (error) {
    return 0;
  }
}

// 格式化文件大小
function formatBytes(bytes) {
  if (bytes === 0) return '0 B';
  
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  
  return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i];
}

// 分析构建产物
function analyzeBuildOutput() {
  log('info', '分析构建产物...');
  
  if (!fs.existsSync(FRONTEND_DIST)) {
    throw new Error('前端构建目录不存在，请先运行 npm run build');
  }
  
  const totalSize = getDirectorySize(FRONTEND_DIST);
  log('info', `构建产物总大小: ${formatBytes(totalSize)}`);
  
  // 分析文件类型
  const fileTypes = {};
  
  function analyzeFiles(dir) {
    const items = fs.readdirSync(dir);
    
    for (const item of items) {
      const itemPath = path.join(dir, item);
      const stats = fs.statSync(itemPath);
      
      if (stats.isDirectory()) {
        analyzeFiles(itemPath);
      } else {
        const ext = path.extname(item).toLowerCase();
        if (!fileTypes[ext]) {
          fileTypes[ext] = { count: 0, size: 0 };
        }
        fileTypes[ext].count++;
        fileTypes[ext].size += stats.size;
      }
    }
  }
  
  analyzeFiles(FRONTEND_DIST);
  
  log('info', '文件类型分析:');
  Object.entries(fileTypes)
    .sort((a, b) => b[1].size - a[1].size)
    .forEach(([ext, data]) => {
      console.log(`  ${ext || '(无扩展名)'}: ${data.count} 文件, ${formatBytes(data.size)}`);
    });
}

// 检查 Go embed 指令
function checkGoEmbedDirective() {
  log('info', '检查 Go embed 指令...');
  
  if (!fs.existsSync(BACKEND_MAIN_FILE)) {
    log('warning', 'main.go 文件不存在，跳过检查');
    return;
  }
  
  const content = fs.readFileSync(BACKEND_MAIN_FILE, 'utf-8');
  
  if (content.includes('//go:embed dist')) {
    log('success', 'Go embed 指令检查通过');
  } else {
    log('warning', '未找到 //go:embed dist 指令，请确保在 main.go 中添加该指令');
    log('info', '示例: //go:embed dist');
  }
}

// 生成集成报告
function generateReport() {
  const reportPath = path.join(__dirname, '../build-report.json');
  const timestamp = new Date().toISOString();
  
  const report = {
    timestamp,
    frontend: {
      framework: 'Vue 3 + TypeScript + Element Plus + Vite',
      distSize: fs.existsSync(FRONTEND_DIST) ? getDirectorySize(FRONTEND_DIST) : 0,
      distPath: FRONTEND_DIST
    },
    backend: {
      framework: 'Go + Gin + SQLite + JWT',
      embedPath: BACKEND_EMBED_DIR,
      mainFile: BACKEND_MAIN_FILE
    },
    integration: {
      success: fs.existsSync(BACKEND_EMBED_DIR),
      method: 'go:embed'
    }
  };
  
  fs.writeFileSync(reportPath, JSON.stringify(report, null, 2));
  log('info', `集成报告已生成: ${reportPath}`);
  
  return report;
}

// 主函数
async function main() {
  try {
    log('info', '开始前端构建产物集成流程...');
    log('info', '前端: Vue 3 + TypeScript + Element Plus + Vite');
    log('info', '后端: Go + Gin + SQLite + JWT');
    console.log('');
    
    // 分析构建产物
    analyzeBuildOutput();
    
    // 清理旧的嵌入目录
    if (fs.existsSync(BACKEND_EMBED_DIR)) {
      log('info', '清理旧的嵌入目录...');
      fs.rmSync(BACKEND_EMBED_DIR, { recursive: true, force: true });
    }
    
    // 复制构建产物
    log('info', '复制前端构建产物到后端...');
    copyDirectory(FRONTEND_DIST, BACKEND_EMBED_DIR);
    
    // 验证复制结果
    const copiedSize = getDirectorySize(BACKEND_EMBED_DIR);
    log('success', `复制完成，共 ${formatBytes(copiedSize)} 数据`);
    
    // 检查 Go embed 指令
    checkGoEmbedDirective();
    
    // 生成集成报告
    const report = generateReport();
    
    console.log('');
    log('success', '🎉 前端构建产物集成完成!');
    
    // 显示后续步骤
    console.log('');
    log('info', '后续步骤:');
    console.log('  1. 构建后端: cd ../Tshell-server && go build -o tshell-server.exe .');
    console.log('  2. 启动服务: ./tshell-server.exe -p 8089');
    console.log('  3. 访问应用: http://localhost:8089');
    
  } catch (error) {
    log('error', error.message);
    process.exit(1);
  }
}

// 处理命令行参数
const args = process.argv.slice(2);
if (args.includes('--help') || args.includes('-h')) {
  console.log(`
前端构建产物集成脚本

用法: node copy-to-backend.js [选项]

选项:
  --help, -h    显示帮助信息

描述:
  将 Vue 3 + Vite 前端构建产物复制到 Go 后端的 embed 目录中，
  以便通过 go:embed 指令将静态文件嵌入到二进制文件中。

前提条件:
  1. 前端已完成构建 (npm run build)
  2. 后端 main.go 文件包含 //go:embed dist 指令

示例:
  npm run build && node scripts/copy-to-backend.js
`);
  process.exit(0);
}

// 运行主函数
main();