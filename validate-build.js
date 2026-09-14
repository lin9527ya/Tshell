#!/usr/bin/env node

/**
 * Rshell 构建验证脚本
 * 功能：验证前端构建、后端集成和服务器的完整性
 * 版本：1.0.0
 */

const fs = require('fs');
const path = require('path');

// 配置
const config = {
  frontendDir: 'Rshell-web',
  backendDir: 'Rshell---A-Cross-Platform-C2',
  serverName: process.platform === 'win32' ? 'rshell-server.exe' : 'rshell-server'
};

// 颜色输出
const colors = {
  reset: '\x1b[0m',
  red: '\x1b[31m',
  green: '\x1b[32m',
  yellow: '\x1b[33m',
  blue: '\x1b[34m',
  magenta: '\x1b[35m'
};

function colorLog(color, message) {
  console.log(`${colors[color]}${message}${colors.reset}`);
}

function logInfo(message) {
  colorLog('blue', `[INFO] ${message}`);
}

function logSuccess(message) {
  colorLog('green', `[SUCCESS] ${message}`);
}

function logWarning(message) {
  colorLog('yellow', `[WARNING] ${message}`);
}

function logError(message) {
  colorLog('red', `[ERROR] ${message}`);
}

function logHeader(message) {
  colorLog('magenta', '='.repeat(50));
  colorLog('magenta', ` ${message}`);
  colorLog('magenta', '='.repeat(50));
}

// 检查文件是否存在
function checkFileExists(filePath, description) {
  if (fs.existsSync(filePath)) {
    logSuccess(`${description}: ✓`);
    return true;
  } else {
    logError(`${description}: ✗ (${filePath})`);
    return false;
  }
}

// 验证前端构建
function validateFrontendBuild() {
  logHeader('验证前端构建');
  
  const frontendDist = path.join(config.frontendDir, 'dist');
  
  if (!fs.existsSync(frontendDist)) {
    logError('前端构建目录不存在，请运行: npm run build');
    return false;
  }
  
  const indexHtml = path.join(frontendDist, 'index.html');
  if (!checkFileExists(indexHtml, 'index.html')) {
    return false;
  }
  
  const staticDir = path.join(frontendDist, 'static');
  if (fs.existsSync(staticDir)) {
    const fileCount = fs.readdirSync(staticDir, { recursive: true }).length;
    logSuccess(`static 目录: ${fileCount} 个文件`);
  } else {
    logWarning('static 目录不存在');
  }
  
  // 检查package.json脚本
  const packageJsonPath = path.join(config.frontendDir, 'package.json');
  if (fs.existsSync(packageJsonPath)) {
    const packageJson = JSON.parse(fs.readFileSync(packageJsonPath, 'utf8'));
    const requiredScripts = ['build', 'build:embed'];
    
    logInfo('检查package.json脚本...');
    for (const script of requiredScripts) {
      if (packageJson.scripts && packageJson.scripts[script]) {
        logSuccess(`脚本 ${script}: ✓`);
      } else {
        logWarning(`脚本 ${script}: ✗`);
      }
    }
  }
  
  return true;
}

// 验证后端集成
function validateBackendIntegration() {
  logHeader('验证后端集成');
  
  const backendDist = path.join(config.backendDir, 'dist');
  
  if (!fs.existsSync(backendDist)) {
    logError('后端嵌入目录不存在');
    return false;
  }
  
  const indexHtml = path.join(backendDist, 'index.html');
  if (!checkFileExists(indexHtml, '后端 index.html')) {
    return false;
  }
  
  // 检查Go文件中的embed指令
  const mainGoPath = path.join(config.backendDir, 'main.go');
  if (fs.existsSync(mainGoPath)) {
    const content = fs.readFileSync(mainGoPath, 'utf8');
    
    if (content.includes('//go:embed dist')) {
      logSuccess('Go embed指令: ✓');
    } else {
      logError('Go embed指令: ✗');
      return false;
    }
    
    if (content.includes('"embed"')) {
      logSuccess('embed包导入: ✓');
    } else {
      logError('embed包导入: ✗');
      return false;
    }
  } else {
    logError('main.go文件不存在');
    return false;
  }
  
  return true;
}

// 验证服务器构建
function validateServerBuild() {
  logHeader('验证服务器构建');
  
  const serverPath = path.join(config.backendDir, config.serverName);
  
  if (!checkFileExists(serverPath, '服务器可执行文件')) {
    return false;
  }
  
  const stats = fs.statSync(serverPath);
  const sizeInMB = (stats.size / (1024 * 1024)).toFixed(2);
  logInfo(`服务器文件大小: ${sizeInMB} MB`);
  
  return true;
}

// 生成验证报告
function generateValidationReport(results) {
  logHeader('验证报告');
  
  const report = {
    timestamp: new Date().toISOString(),
    platform: process.platform,
    node_version: process.version,
    results: results,
    overall_status: Object.values(results).every(Boolean) ? 'PASS' : 'FAIL'
  };
  
  // 保存报告
  const reportPath = 'validation-report.json';
  fs.writeFileSync(reportPath, JSON.stringify(report, null, 2));
  
  // 显示结果
  console.log('\\n验证结果:');
  for (const [test, result] of Object.entries(results)) {
    const status = result ? '✅ PASS' : '❌ FAIL';
    console.log(`  ${test}: ${status}`);
  }
  
  console.log(`\\n总体状态: ${report.overall_status}`);
  logInfo(`详细报告已保存: ${reportPath}`);
  
  return report.overall_status === 'PASS';
}

// 显示使用建议
function showUsageRecommendations(success) {
  logHeader('使用建议');
  
  if (success) {
    colorLog('green', '🎉 所有验证通过！系统已准备就绪');
    console.log('');
    console.log('启动生产服务器:');
    colorLog('blue', `  cd ${config.backendDir}`);
    colorLog('blue', `  ./${config.serverName}`);
    console.log('');
    console.log('访问应用:');
    colorLog('blue', '  http://localhost:8089');
  } else {
    colorLog('red', '❌ 验证失败，请检查以上错误');
    console.log('');
    console.log('常见问题解决:');
    console.log('  1. 前端构建失败 -> 运行: npm run build');
    console.log('  2. 后端集成失败 -> 运行: node scripts/copy-to-backend.js');
    console.log('  3. 服务器编译失败 -> 运行: go build -o rshell-server .');
  }
}

// 主函数
function main() {
  try {
    logHeader('Rshell 构建验证工具 v1.0.0');
    
    const results = {
      frontend_build: false,
      backend_integration: false,
      server_build: false
    };
    
    // 执行验证步骤
    results.frontend_build = validateFrontendBuild();
    results.backend_integration = validateBackendIntegration();
    results.server_build = validateServerBuild();
    
    // 生成报告和建议
    const success = generateValidationReport(results);
    showUsageRecommendations(success);
    
    process.exit(success ? 0 : 1);
    
  } catch (error) {
    logError(`验证过程中发生错误: ${error.message}`);
    if (process.env.DEBUG) {
      console.error(error.stack);
    }
    process.exit(1);
  }
}

// 处理命令行参数
if (process.argv.includes('--help') || process.argv.includes('-h')) {
  console.log(`
Rshell 构建验证工具

用法: node validate-build.js [选项]

选项:
  --help, -h    显示帮助信息

功能:
  1. 验证前端构建完整性
  2. 验证后端集成配置
  3. 验证服务器编译结果

环境变量:
  DEBUG=1       显示详细错误信息
`);
  process.exit(0);
}

// 运行主函数
if (require.main === module) {
  main();
}

module.exports = {
  validateFrontendBuild,
  validateBackendIntegration,
  validateServerBuild
};