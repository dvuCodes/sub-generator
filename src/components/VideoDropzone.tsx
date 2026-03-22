import { useCallback, useState } from "react";

interface VideoDropzoneProps {
  selectedFile: string | null;
  onFileSelect: (path: string) => void;
  disabled?: boolean;
}

const SUPPORTED_EXTENSIONS = [
  ".mp4",
  ".mkv",
  ".avi",
  ".mov",
  ".webm",
  ".flv",
  ".wmv",
  ".m4v",
];

export function VideoDropzone({
  selectedFile,
  onFileSelect,
  disabled,
}: VideoDropzoneProps) {
  const [isDragOver, setIsDragOver] = useState(false);

  const handleBrowse = useCallback(async () => {
    const { open: openDialog } = await import("@tauri-apps/plugin-dialog");
    const selected = await openDialog({
      multiple: false,
      filters: [
        {
          name: "Video Files",
          extensions: SUPPORTED_EXTENSIONS.map((extension) => extension.slice(1)),
        },
      ],
    });

    if (selected) {
      onFileSelect(selected as string);
    }
  }, [onFileSelect]);

  const handleDragOver = useCallback(
    (event: React.DragEvent) => {
      event.preventDefault();
      if (!disabled) {
        setIsDragOver(true);
      }
    },
    [disabled]
  );

  const handleDragLeave = useCallback(() => {
    setIsDragOver(false);
  }, []);

  const handleDrop = useCallback(
    (event: React.DragEvent) => {
      event.preventDefault();
      setIsDragOver(false);
      if (disabled) {
        return;
      }

      const file = event.dataTransfer.files[0];
      if (!file) {
        return;
      }

      const extension = "." + file.name.split(".").pop()?.toLowerCase();
      if (!SUPPORTED_EXTENSIONS.includes(extension)) {
        return;
      }

      const droppedPath = (file as File & { path?: string }).path;
      if (droppedPath) {
        onFileSelect(droppedPath);
      }
    },
    [disabled, onFileSelect]
  );

  const fileName = selectedFile?.split(/[/\\]/).pop() ?? null;

  return (
    <div
      className={`
        cursor-pointer rounded-xl border-2 border-dashed p-8 text-center transition-all
        ${isDragOver ? "border-blue-400 bg-blue-400/10" : "border-gray-700 hover:border-gray-500"}
        ${disabled ? "cursor-not-allowed opacity-50" : ""}
        ${selectedFile ? "border-green-500/50 bg-green-500/5" : ""}
      `}
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
      onClick={disabled ? undefined : handleBrowse}
    >
      {selectedFile ? (
        <div className="space-y-2">
          <div className="text-xs uppercase tracking-[0.35em] text-green-400">
            Selected
          </div>
          <p className="font-medium text-green-400">{fileName}</p>
          <p className="text-sm text-gray-500">
            Click to choose a different file
          </p>
        </div>
      ) : (
        <div className="space-y-2">
          <div className="text-xs uppercase tracking-[0.35em] text-gray-500">
            Input Video
          </div>
          <p className="font-medium text-gray-300">
            Click to select a video file
          </p>
          <p className="text-sm text-gray-500">
            Drop works when the desktop runtime exposes a file path.
          </p>
          <p className="text-xs text-gray-600">
            Supports: {SUPPORTED_EXTENSIONS.join(", ")}
          </p>
        </div>
      )}
    </div>
  );
}
