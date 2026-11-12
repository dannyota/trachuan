export function isNodeCode(value: string): boolean {
  return /^[1-9][0-9]{0,2}(\.[1-9][0-9]{0,3}(\.[pcx][1-9][0-9]{0,3}(\.[cmx][1-9][0-9]{0,3})?)?)?$/.test(value)
}
