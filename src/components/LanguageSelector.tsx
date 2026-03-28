import { useDeferredValue, useMemo, useState } from "react";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { LanguageOption } from "@/lib/types";

const EMPTY_LANGUAGES: LanguageOption[] = [];

interface LanguageSelectorProps {
  sourceLang: string;
  targetLang: string;
  onSourceChange: (lang: string) => void;
  onTargetChange: (lang: string) => void;
  sourceLanguages?: LanguageOption[];
  targetLanguages?: LanguageOption[];
  translationStatus?: string;
  disabled?: boolean;
  targetDisabled?: boolean;
}

function filterLanguages(
  languages: LanguageOption[],
  query: string,
  selectedCode: string
) {
  if (!query.trim()) {
    return languages;
  }

  const selected = languages.find((language) => language.code === selectedCode);
  const filtered = languages.filter((language) =>
    `${language.name} ${language.code}`
      .toLowerCase()
      .includes(query.trim().toLowerCase())
  );

  if (selected && !filtered.some((language) => language.code === selected.code)) {
    return [selected, ...filtered];
  }

  return filtered;
}

export function LanguageSelector({
  sourceLang,
  targetLang,
  onSourceChange,
  onTargetChange,
  sourceLanguages,
  targetLanguages,
  translationStatus,
  disabled,
  targetDisabled = false,
}: LanguageSelectorProps) {
  const [sourceQuery, setSourceQuery] = useState("");
  const [targetQuery, setTargetQuery] = useState("");
  const deferredSourceQuery = useDeferredValue(sourceQuery);
  const deferredTargetQuery = useDeferredValue(targetQuery);
  const availableSourceLanguages = sourceLanguages ?? EMPTY_LANGUAGES;
  const availableTargetLanguages = useMemo(
    () => [{ code: "", name: "Select target language" }, ...(targetLanguages ?? [])],
    [targetLanguages]
  );
  const filteredSourceLanguages = useMemo(
    () => filterLanguages(availableSourceLanguages, deferredSourceQuery, sourceLang),
    [availableSourceLanguages, deferredSourceQuery, sourceLang]
  );
  const filteredTargetLanguages = useMemo(
    () => filterLanguages(availableTargetLanguages, deferredTargetQuery, targetLang),
    [availableTargetLanguages, deferredTargetQuery, targetLang]
  );

  return (
    <div className="space-y-3">
      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-1.5">
          <Label className="text-xs text-muted-foreground uppercase tracking-wider">
            Source
          </Label>
          <input
            value={sourceQuery}
            onChange={(event) => setSourceQuery(event.target.value)}
            placeholder="Search source language"
            disabled={disabled}
            className="h-9 w-full border border-border bg-background px-3 text-xs outline-none transition-colors placeholder:text-muted-foreground focus:border-primary disabled:cursor-not-allowed disabled:opacity-50"
          />
          <Select
            value={sourceLang}
            onValueChange={onSourceChange}
            disabled={disabled}
          >
            <SelectTrigger className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {filteredSourceLanguages.map((lang) => (
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
          <input
            value={targetQuery}
            onChange={(event) => setTargetQuery(event.target.value)}
            placeholder="Search target language"
            disabled={disabled || targetDisabled || availableTargetLanguages.length <= 1}
            className="h-9 w-full border border-border bg-background px-3 text-xs outline-none transition-colors placeholder:text-muted-foreground focus:border-primary disabled:cursor-not-allowed disabled:opacity-50"
          />
          <Select
            value={targetLang || "__none__"}
            onValueChange={(val) => onTargetChange(val === "__none__" ? "" : val)}
            disabled={disabled || targetDisabled || availableTargetLanguages.length <= 1}
          >
            <SelectTrigger className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {filteredTargetLanguages.map((lang) => (
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
