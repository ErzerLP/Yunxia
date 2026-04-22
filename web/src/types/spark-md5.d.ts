declare module 'spark-md5' {
  class SparkArrayBuffer {
    append(data: ArrayBuffer): void
    end(raw?: boolean): string
    reset(): void
    getState(): { buff: Uint8Array; length: number; hash: number[] }
    setState(state: { buff: Uint8Array; length: number; hash: number[] }): void
    destroy(): void
  }

  export { SparkArrayBuffer as ArrayBuffer }

  interface SparkMD5Static {
    ArrayBuffer: typeof SparkArrayBuffer
    hash(data: string, raw?: boolean): string
    hashBinary(data: string, raw?: boolean): string
    hashArrayBuffer(data: ArrayBuffer, raw?: boolean): string
  }

  const SparkMD5: SparkMD5Static
  export default SparkMD5
}
