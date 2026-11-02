import {
  SystemService,
  type EnvironmentDTO,
  type LogEntryDTO,
} from '../../bindings/audio-ui/internal/handler'

export type { EnvironmentDTO, LogEntryDTO }

export async function fetchEnvironment(refresh = false): Promise<EnvironmentDTO> {
  return refresh
    ? await SystemService.RefreshEnvironment()
    : await SystemService.GetEnvironment()
}

export async function fetchRecentLogs(limit = 150): Promise<LogEntryDTO[]> {
  return (await SystemService.GetRecentLogs(limit)) ?? []
}
