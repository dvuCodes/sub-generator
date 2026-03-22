import { useState, useCallback } from "react";

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
    // Use Tauri dialog to pick a file
    const { open: openDialog } = await import("@tauri-apps/plugin-dialog");
    const selected = await openDialog({
      multiple: false,
      filters: [
        {
          name: "Video Files",
          extensions: SUPPORTED_EXTENSIONS.map((ext) => ext.slice(1)),
        },
      ],
    });
    if (selected) {
      onFileSelect(selected as string);
    }
  }, [onFileSelect]);

  const handleDragOver = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      if (!disabled) setIsDragOver(true);
    },
    [disabled]
  );

  const handleDragLeave = useCallback(() => {
    setIsDragOver(false);
  }, []);

  const handleDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      setIsDragOver(false);
      if (disabled) return;

      const files = e.dataTransfer.files;
      if (files.length > 0) {
        const file = files[0];
        const ext = "." + file.name.split(".").pop()?.toLowerCase();
        if (SUPPORTED_EXTENSIONS.includes(ext)) {
          // Note: In Tauri, drag-and-drop gives us a path via the drop event
          // For now, use the file browser as the primary method
          onFileSelect(file.name);
        }
      }
    },
    [disabled, onFileSelect]
  );

  const fileName = selectedFile?.split(/[/\\]/).pop() ?? null;

  return (
    <div
      className={`
        border-2 border-dashed rounded-xl p-8 text-center transition-all cursor-pointer
        ${isDragOver ? "border-blue-400 bg-blue-400/10" : "border-gray-700 hover:border-gray-500"}
        ${disabled ? "opacity-50 cursor-not-allowed" : ""}
        ${selectedFile ? "border-green-500/50 bg-green-500/5" : ""}
      `}
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
      onClick={disabled ? undefined : handleBrowse}
    >
      {selectedFile ? (
        <div className="space-y-2">
          <div className="text-4xl">🎬</div>
          <p className="text-green-400 font-medium">{fileName}</p>
          <p className="text-gray-500 text-sm">Click to change file</p>
        </div>
      ) : (
        <div className="space-y-2">
          <div className="text-4xl">📁</div>
          <p className="text-gray-300 font-medium">
            Click to select a video file
          </p>
          <p className="text-gray-500 text-sm">
            Supports: {SUPPORTED_EXTENSIONS.join(", ")}
          </p>
        </div>
      )}
    </div>
  );
}
