import request from '@/utils/request'

// 报告 API(威胁指标 / 社会工程 / 战术 TTP)
const reportApi = {
  // 威胁指标报告
  getIOC() {
    return request({
      url: '/report/ioc',
      method: 'get',
    })
  },

  // 社会工程报告(基于 WebDelivery 访问日志)
  getSocial() {
    return request({
      url: '/report/social',
      method: 'get',
    })
  },

  // 战术 TTP 报告(MITRE 映射统计)
  getTTP() {
    return request({
      url: '/report/ttp',
      method: 'get',
    })
  },
}

export default reportApi
