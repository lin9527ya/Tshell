/**
 * WebDelivery 列表字段归一化。
 *
 * 后端 `WebDelivery` 结构体的 JSON tag 是 snake_case
 * （listening_port / os / arch / listener_config / status ...），
 * 而部分页面历史代码按 PascalCase 访问。统一在这里补齐两套字段，
 * 避免下拉框出现 “端口 undefined | undefined_undefined”。
 */
export function normalizeWebDelivery(item: any) {
  const pick = (camel: string, snake: string) => item?.[camel] ?? item?.[snake]
  return {
    ...item,
    ID: Number(pick('ID', 'id') ?? 0),
    ListeningPort: String(pick('ListeningPort', 'listening_port') ?? ''),
    OS: String(pick('OS', 'os') ?? ''),
    Arch: String(pick('Arch', 'arch') ?? ''),
    ListenerConfig: String(pick('ListenerConfig', 'listener_config') ?? ''),
    Status: Number(pick('Status', 'status') ?? 0),
    ServerAddress: String(pick('ServerAddress', 'server_address') ?? ''),
    StagerURL: String(pick('StagerURL', 'stager_url') ?? ''),
    ShellcodeURL: String(pick('ShellcodeURL', 'shellcode_url') ?? ''),
    PublicKey: String(pick('PublicKey', 'public_key') ?? ''),
    BindMode: String(pick('BindMode', 'bind_mode') ?? 'shared'),
    FileName: String(pick('FileName', 'file_name') ?? ''),
    SaveFilename: String(pick('SaveFilename', 'save_filename') ?? ''),
    CryptoMode: String(pick('CryptoMode', 'crypto_mode') ?? ''),
  }
}

export function normalizeWebDeliveryList(list: any[]): ReturnType<typeof normalizeWebDelivery>[] {
  return (list || []).map(normalizeWebDelivery)
}
