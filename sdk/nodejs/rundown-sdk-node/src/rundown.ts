import { Worker } from "./worker.js";
import type { Handler, QueueConfig, WorkerConfig } from "./types.js";

let instance: Worker | null = null;

function getInstance(config?: WorkerConfig): Worker {
  if (!instance) {
    instance = new Worker(config ?? { host: "http://localhost:8181" });
  }
  return instance;
}

export function queue<T = unknown>(
  config: QueueConfig,
  handler: Handler<T>
): void {
  getInstance().register(config, handler);
}

export async function run(config?: WorkerConfig): Promise<void> {
  await getInstance(config).start();
}
