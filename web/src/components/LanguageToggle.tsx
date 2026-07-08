import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { useLang } from '@/i18n'

/** Header control that flips the UI language between EN and RU in one click. */
export function LanguageToggle() {
  const { lang, toggle, t } = useLang()
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          onClick={toggle}
          aria-label={t('lang.aria')}
        >
          <span className="text-[11px] font-semibold tabular-nums">
            {lang.toUpperCase()}
          </span>
        </Button>
      </TooltipTrigger>
      <TooltipContent>
        {lang === 'en' ? t('lang.switchToRu') : t('lang.switchToEn')}
      </TooltipContent>
    </Tooltip>
  )
}
