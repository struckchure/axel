export const url = process.env.BENCH_DATABASE_URL ?? 'postgresql://benchmark:benchmark@127.0.0.1:55439/axel_benchmark';
export function assertBenchmarkDatabase() {
  if (new URL(url).pathname !== '/axel_benchmark') throw new Error('Use a dedicated database named axel_benchmark; seed resets benchmark tables.');
}
