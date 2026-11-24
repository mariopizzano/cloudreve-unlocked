import React, { useEffect, useRef, useState } from "react";
import Base, { RetryOption } from "./core/uploader/base.ts";
import { useSnackbar } from "notistack";

export function useUpload(uploader: Base) {
  const startLoadedRef = useRef(0);
  const baselineRef = useRef<{ startTime: number; loaded: number } | null>(null);
  const [status, setStatus] = useState(uploader.status);
  const [error, setError] = useState(uploader.error);
  const [progress, setProgress] = useState(uploader.progress);
  const { enqueueSnackbar } = useSnackbar();

  useEffect(() => {
    /* eslint-disable @typescript-eslint/no-empty-function */
    uploader.subscribe({
      onTransition: (newStatus) => {
        setStatus(newStatus);
      },
      onError: (err) => {
        setError(err);
        setStatus(uploader.status);
      },
      onProgress: (data) => {
        setProgress(data);
      },
      onMsg: (msg, color) => {
        enqueueSnackbar(msg, { variant: color });
      },
    });
  }, []);

  // 获取上传速度
  const [speed, speedAvg] = React.useMemo(() => {
    if (progress == undefined || progress.total == null || progress.total.loaded == null) return [0, 0];

    const now = Date.now();
    const startTime = uploader.startTime || 0;

    // On a new upload, retry or resume, bytes that are already present must not be
    // counted as transferred: otherwise a resumed task reports GB/s for a moment.
    if (baselineRef.current == null || baselineRef.current.startTime !== startTime) {
      baselineRef.current = { startTime, loaded: progress.total.loaded };
      startLoadedRef.current = progress.total.loaded;
      uploader.lastTime = now;
      return [0, 0];
    }

    const duration = Math.max((now - (uploader.lastTime || now)) / 1000, 0.2);
    const durationTotal = Math.max((now - (startTime || now)) / 1000, 0.2);
    const baseline = baselineRef.current.loaded;
    const res =
      progress.total.loaded > startLoadedRef.current
        ? Math.floor((progress.total.loaded - startLoadedRef.current) / duration)
        : 0;
    const resAvg = Math.max(Math.floor((progress.total.loaded - baseline) / durationTotal), 0);

    startLoadedRef.current = progress.total.loaded;
    uploader.lastTime = now;
    return [res, resAvg];
  }, [progress?.total.loaded]);

  const retry = (opt?: RetryOption) => {
    uploader.retry(opt);
  };

  return { status, error, progress, speed, speedAvg, retry };
}
