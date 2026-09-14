import { ElMessage, ElMessageBox } from 'element-plus';
import axios from 'axios';
import { useUserStore } from '@/stores/user';
// import {useAxiosConfigStore} from '@/stores/server';
import router from '@/router';
import axiosRetry from 'axios-retry';

// 【修复】移除全局 cancel 令牌：单令牌被每个请求覆盖会取消先发的进行中请求
// （如同时加载审计日志+上传文件会互相打断）。需要取消时组件内自行创建 CancelToken。
let service = axios.create({
  // baseURL: axiosConfig.baseURL,
  baseURL: '/api',
  timeout: 90 * 1000, // 设置默认超时为90秒，单个API可以覆盖此设置
  withCredentials: true
});


// 请求重试配置 - 优化重试策略
axiosRetry(service, {
  retries: 2, // 减少重试次数以避免过多重试
  retryDelay: (retryCount) => retryCount * 2000, // 递增延迟
  shouldResetTimeout: true,
  retryCondition: (error) => {
    // 对截图API的请求，永不重试，因为它有自己的超时和用户提示逻辑
    // 使用 includes 匹配，更加健壮
    if (error.config?.url?.includes('/client/screenshot1')) {
      console.log('检测到截图API超时，根据策略，不进行重试。');
      return false;
    }
    // 只在特定错误时重试
    return ['ECONNABORTED', 'ERR_NETWORK', 'ENOTFOUND', 'ECONNRESET'].includes(error.code!) ||
           (error.response && [408, 502, 503, 504].includes(error.response.status)) ||
           false // 确保始终返回 boolean 值
  }
});

// 移除截图请求的重复取消机制，允许排队请求
// 每次请求在 header 中带上 token
service.interceptors.request.use((config) => {
  const userStore = useUserStore();
  const token = userStore.getToken();
  
  // 添加请求开始时间用于性能监控
  (config as any).metadata = { startTime: new Date() };
  
  if (token) {
    config.headers.Authorization2 = 'Bearer ' + token;
  } else {
    // 未登录请求（如公开路由）
  }

  return config;
});

// 拦截每一次响应，判断是否 token 失效
service.interceptors.response.use(
    (response) => {
      // 添加更详细的日志记录
  if (response.config.url === '/client/screenshot1') {
    const startTime = (response.config as any).metadata?.startTime as Date || new Date();
    const duration = new Date().getTime() - startTime.getTime();
    console.log(`截图API请求完成，耗时${duration}毫秒`);
  }

      const userStore = useUserStore();
      const responseData = response.data;

      // 首先处理HTTP层面的认证错误
      if (response.status === 401) {
          ElMessage({
              message: '登录已过期，请重新登录',
              type: 'warning',
              duration: 5000
          });
          userStore.logout();
          handleError('登录已过期，请重新登录');
          return Promise.reject(new Error('登录已过期'));
      }

      // 检查HTTP状态码是否成功
      if (response.status !== 200) {
          const errorMsg = responseData.error || responseData.msg || '接口异常';
          ElMessage({
              message: errorMsg,
              type: 'error',
              duration: 5000
          });
          return Promise.reject(new Error(errorMsg));
      }

      // 检查响应体中的业务状态码或标志
      // 这部分兼容多种后端返回格式
      if (
          (responseData.code && responseData.code !== 200 && responseData.code !== 401 && responseData.code !== 402) ||
          // 【契约修复】仅数值型 status 视为业务码；/client/smart-offline-check 返回
          // {status:"online|offline|possibly_busy|likely_offline"} 字符串状态，不应被判为业务错误
          (typeof responseData.status === 'number' && responseData.status !== 200) ||
          (typeof responseData.success !== 'undefined' && !responseData.success)
      ) {
          // 特殊处理认证失败的业务码
          if (responseData.code === 401 || responseData.code === 402) {
              ElMessage({
                  message: responseData.msg || '页面长时间未使用，请重新登录',
                  type: 'warning',
                  duration: 5000
              });
              userStore.logout();
              handleError('页面长时间未使用，请重新登录');
              return Promise.reject(new Error(responseData.msg || 'Error'));
          }

          // 其他业务错误（兼容后端 {status:400, data:"错误文案"} 包装）
          const errorMsg = responseData.error || responseData.msg || responseData.message || (typeof responseData.data === 'string' ? responseData.data : '') || '操作失败';
          ElMessage({
              message: errorMsg,
              type: 'error',
              duration: 5000
          });
          return Promise.reject(new Error(errorMsg));
      }

      // 所有检查通过，返回完整的响应对象以保持类型一致性

      // 【统一响应格式】后端统一契约 {success, code, message, data, timestamp}，
      // 归一化为前端习惯的 {status: HTTP状态, data, message}：
      // 存量组件大量使用 res.data.status === 200 / res.data.data，保持兼容零改动。
      const rd = response.data;
      if (rd && typeof rd === 'object' && !Array.isArray(rd) &&
          'code' in rd && 'success' in rd && !('status' in rd)) {
        response.data = {
          status: response.status,
          data: rd.data,
          message: rd.message || (rd.success ? '操作成功' : '操作失败'),
          success: rd.success,
          code: rd.code,
          timestamp: rd.timestamp
        };
      }
      return response;
    },
    async (error) => {
      // 添加更详细的错误日志记录
  if (error.config?.url === '/client/screenshot1') {
    const startTime = (error.config as any).metadata?.startTime as Date || new Date();
    const duration = new Date().getTime() - startTime.getTime();
    console.error(`截图API请求错误，耗时${duration}毫秒，错误信息：`, error.message);
  }
  
      if (axios.isCancel(error)) {
        console.log('Request canceled', error.message);
        return Promise.reject(error);
      }
      
      const userStore = useUserStore();

      // 后台数据源静默失败：监控帧拉取 408/429/404 与拓扑图 403/失败均属正常状态，
      // 由 PivotGraph / ClientMonitor 自行回退或消化，不弹全局错误提示
      if (error.config?.url?.includes('/client/monitor/frame') || error.config?.url?.includes('/graph/data') || error.config?.url?.includes('/connection/uid/') || error.config?.url?.includes('/client/smart-offline-check')) {
        return Promise.reject(error);
      }

      if (error.response) {
        // 服务器返回了错误响应
        if (error.response.status === 401) {
          // 登录失败也是 401（后端 AbortWithError）——不能当成会话过期处理，
          // 否则会误弹"登录已过期"并触发无意义的 refresh 流程（LoginView 会再弹真实错误）
          if (error.config?.url?.includes('/users/login')) {
            ElMessage.error(error.response.data?.message || '用户名或密码错误');
            return Promise.reject(error);
          }
          // 只有非验证请求才显示错误信息，避免路由守卫的token验证失败显示错误
            const isAuthValidation = error.config?.url?.includes('/auth/check');
          
          if (!isAuthValidation) {
            // F5: 刷新重试仅一次——原无限重试在 token 仍被拒时形成刷新→401→刷新的循环风暴
            const alreadyRetried = (error.config as any)?._retry;
            if (alreadyRetried) {
              ElMessage.error('登录已过期，请重新登录');
              userStore.logout();
              return Promise.reject(error);
            }
            // 尝试刷新token
            const refreshSuccess = await userStore.refreshToken();
            if (refreshSuccess && error.config) {
              // token刷新成功，重试原请求（仅一次）
              (error.config as any)._retry = true;
              error.config.headers.Authorization2 = 'Bearer ' + userStore.getToken();
              return service.request(error.config);
            } else {
              // token刷新失败或不可刷新
              ElMessage.error('登录已过期，请重新登录');
              userStore.logout();
            }
          } else {
            // 路由守卫的验证失败，不显示错误信息
            console.log('路由守卫的JWT令牌验证失败，不显示错误信息');
            userStore.logout();
          }
        } else if (error.response.status === 404 || error.response.data?.code === 404) {
          // F6: API 层 404 只提示不跳转（原强制跳 /404 把用户踢出当前页）
          ElMessage.error('请求的资源不存在');
        } else if (error.response.status === 410) {
          // F6: 已下线接口提示（原落入 else 显示误导性的「服务器异常」）
          ElMessage.error('该功能已下线');
        } else if (error.response.status === 500) {
          // 针对隧道代理的500错误，不跳转，只显示错误信息
          const errorMsg = error.response.data?.error || error.response.data?.message || '服务器出错了';
          ElMessage.error(errorMsg);
          // 不跳转，让用户留在当前页面
        } else if (error.response.status === 429) {
          // 【修复】限流是临时状态，不再强制登出；轮询触限只提示并短暂放慢
          ElMessage.error(error.response.data?.msg || '请求过于频繁，请稍后再试');
        } else {
          ElMessage.error('服务器异常');
        }
      } else if (error.request) {
        // 网络错误或服务器无响应
        ElMessage.error('网络错误，请检查服务器连接');
      } else {
        // 其他错误
        ElMessage.error('请求失败：' + (error.message || '未知错误'));
      }
      
      return Promise.reject(error);
    }
);

function handleError(msg:any) {
  const userStore = useUserStore();
  const userName = userStore.getUserName();
  if (userName) {
    ElMessageBox.confirm(msg, {
      confirmButtonText: '再次登录',
      cancelButtonText: '取消',
      type: 'warning'
    }).then(() => {
      userStore.logout();
    });
  }
}

// 取消函数保留导出（兼容旧调用），但已无全局令牌可取消；
// 组件需要取消时请使用 axios.CancelToken 自行管理
export const cancelRequest = () => {
  // 全局取消机制已移除（见文件头注释）
};

export default service;
