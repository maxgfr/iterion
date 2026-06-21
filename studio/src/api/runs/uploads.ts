// Extracted from api/runs.ts to keep that file focused.
// Server info + attachment staging: getServerInfo for the BackendStatusPill /
// LaunchView, uploadAttachment for the staged-upload XHR (with progress).

import type { ServerInfo, StagedUpload } from "../types";
import { BASE_URL, request } from "./client";
import type { UploadOptions } from "./types";

/** GET /api/server/info — mode, version, upload limits. */
export async function getServerInfo(): Promise<ServerInfo> {
  return request("/server/info");
}

/**
 * POST /api/runs/uploads — upload a single attachment to the server's
 * staging area. Uses XMLHttpRequest because fetch() in browsers does
 * not yet expose request-side upload progress (ReadableStream upload
 * is half-duplex and Chromium-only).
 */
export function uploadAttachment(
  file: File,
  opts: UploadOptions = {},
): Promise<StagedUpload> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    const fd = new FormData();
    fd.append("file", file, file.name);
    if (opts.declaredMime) fd.append("declared_mime", opts.declaredMime);

    xhr.open("POST", `${BASE_URL}/runs/uploads`, true);
    xhr.responseType = "json";

    xhr.upload.onprogress = (evt) => {
      if (opts.onProgress && evt.lengthComputable) {
        opts.onProgress(evt.loaded, evt.total);
      }
    };
    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        resolve(xhr.response as StagedUpload);
      } else {
        const body = xhr.response;
        const message =
          body && typeof body === "object" && "error" in body
            ? (body as { error: string }).error
            : `HTTP ${xhr.status}`;
        reject(new Error(message));
      }
    };
    xhr.onerror = () => reject(new Error("network error"));
    xhr.onabort = () => reject(new DOMException("aborted", "AbortError"));

    if (opts.signal) {
      if (opts.signal.aborted) {
        xhr.abort();
        return;
      }
      opts.signal.addEventListener("abort", () => xhr.abort(), { once: true });
    }

    xhr.send(fd);
  });
}
