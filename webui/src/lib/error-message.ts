import { ApiError } from "./api-client";

type TranslateFn = (text: string, options?: Record<string, unknown>) => string;

export function formatApiErrorMessage(error: unknown, t: TranslateFn): string {
  if (error instanceof ApiError) {
    const message = error.message ? t(error.message) : t("未知错误");
    if (!error.code || error.code === "INTERNAL") {
      return message;
    }
    return `${error.code}: ${message}`;
  }
  if (error instanceof Error) {
    return error.message ? t(error.message) : t("未知错误");
  }
  return t("未知错误");
}
