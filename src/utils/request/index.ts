import { ElMessage, ElMessageBox } from 'element-plus';
import axios from 'axios';
import { useUserStore } from '@/stores/user';
// import {useAxiosConfigStore} from '@/stores/server';
import router from '@/router';
import axiosRetry from 'axios-retry';

const CancelToken = axios.CancelToken;
let cancel :any= null;

// const axiosConfig = useAxiosConfigStore();

let service = axios.create({
  // baseURL: axiosConfig.baseURL,
  baseURL: '/api',
  timeout: 10 * 60 * 1000,
  withCredentials: true
});


// 请求重试配置
axiosRetry(service, {
  retries: 3,
  retryDelay: () => 1000,
  shouldResetTimeout: true,
  retryCondition: (error) => ['ECONNABORTED', 'ERR_NETWORK'].includes(error.code!)
});

// 每次请求在 header 中带上 token
service.interceptors.request.use((config) => {
  const userStore = useUserStore();
  if (userStore.getToken()) {
    config.headers.Authorization2 = 'Bearer ' + userStore.getToken();
  }

  // 创建一个新的取消令牌
  config.cancelToken = new CancelToken(c => {
    cancel = c;
  });

  return config;
});

// 拦截每一次响应，判断是否 token 失效
service.interceptors.response.use(
    (response) => {
      const res = response;
      const userStore = useUserStore();
      if (res.data.code === 401 || res.data.code === 402 || res.status === 401) {
        ElMessage({
          message: res.data.msg || '页面长时间未使用，请重新登录',
          type: 'warning',
          duration: 5000
        });
        userStore.logout();
        handleError('页面长时间未使用，请重新登录');
        return Promise.reject(new Error(res.data.msg || 'Error'));
      } else if (res.status !== 200 || (res.data.code !== 200 && 'code' in res.data)) {
        let data = '';
        if (res.data.data && res.data.data !== '') {
          data = res.data.data;
        } else if (res.data.msg && res.data.msg !== 'error') {
          data = res.data.msg;
        }
        ElMessage({
          message: data || '接口异常',
          type: 'error',
          duration: 5000
        });
        return Promise.reject(new Error(data || '接口异常'));
      } else {
        return res;
      }
    },
    (error) => {
      if (axios.isCancel(error)) {
        console.log('Request canceled', error.message);
      } else {
        if (error.response && (error.response.status === 401 || error.response.data.code === 401)) {
          ElMessage.error('非法访问');
          router.push('/login').then();
        } else if (error.response && (error.response.status === 404 || error.response.data.code === 404)) {
          ElMessage.error('找不到页面');
          router.push('/404').then();
        } else if (error.response && error.response.status === 500) {
          ElMessage.error('服务器出错了');
          setTimeout(() => {
            window.location.href = '/login';
          }, 1000);
        } else if (error.response && error.response.status === 429) {
          ElMessage.error(error.response.data.msg);
          setTimeout(() => {
            window.location.href = '/login';
          }, 1000);
        } else {
          handleError('服务器异常');
          setTimeout(() => {
            window.location.href = '/login';
          }, 1000);
        }
      }
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

// 导出取消函数
export const cancelRequest = () => {
  if (cancel) {
    cancel('Operation canceled by the user.');
  }
};

export default service;