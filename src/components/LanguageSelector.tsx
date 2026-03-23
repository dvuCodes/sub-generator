import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

interface LanguageOption {
  code: string;
  name: string;
}

interface LanguageSelectorProps {
  sourceLang: string;
  targetLang: string;
  onSourceChange: (lang: string) => void;
  onTargetChange: (lang: string) => void;
  sourceLanguages?: LanguageOption[];
  targetLanguages?: LanguageOption[];
  translationStatus?: string;
  disabled?: boolean;
}

const LANGUAGES: LanguageOption[] = [
  { code: "auto", name: "Auto-detect" },
  { code: "en", name: "English" },
  { code: "ja", name: "Japanese" },
];

const TARGET_LANGUAGES = [
  { code: "", name: "No translation" },
  { code: "en", name: "English" },
  { code: "ja", name: "Japanese" },
];

export function LanguageSelector({
  sourceLang,
  targetLang,
  onSourceChange,
  onTargetChange,
  sourceLanguages,
  targetLanguages,
  translationStatus,
  disabled,
}: LanguageSelectorProps) {
  const availableSourceLanguages = sourceLanguages?.length
    ? sourceLanguages
    : LANGUAGES;
  const availableTargetLanguages = targetLanguages?.length
    ? [{ code: "", name: "No translation" }, ...targetLanguages]
    : TARGET_LANGUAGES;

  return (
    <div className="space-y-3">
      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-1.5">
          <Label className="text-xs text-muted-foreground uppercase tracking-wider">
            Source
          </Label>
          <Select
            value={sourceLang}
            onValueChange={onSourceChange}
            disabled={disabled}
          >
            <SelectTrigger className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {availableSourceLanguages.map((lang) => (
                <SelectItem key={lang.code} value={lang.code}>
                  {lang.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="space-y-1.5">
          <Label className="text-xs text-muted-foreground uppercase tracking-wider">
            Target
          </Label>
          <Select
            value={targetLang || "__none__"}
            onValueChange={(val) => onTargetChange(val === "__none__" ? "" : val)}
            disabled={disabled}
          >
            <SelectTrigger className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {availableTargetLanguages.map((lang) => (
                <SelectItem key={lang.code || "__none__"} value={lang.code || "__none__"}>
                  {lang.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>

      {translationStatus ? (
        <p className="text-xs text-muted-foreground">{translationStatus}</p>
      ) : null}
    </div>
  );
}
