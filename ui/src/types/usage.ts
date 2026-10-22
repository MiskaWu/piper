// 用量統計相關型別

export interface UsageTier {
  utilization: number
  resetsAt?: string
}

export interface Usage {
  source?: string
  stale?: boolean
  fiveHour?: UsageTier
  sevenDay?: UsageTier
  retryAfterSec?: number
}

export interface DashboardStats {
  usage?: {
    avgFiveHourUtil?: number
    maxFiveHourUtil?: number
    daysAbove80Pct?: number
    daysAbove50Pct?: number
    totalDays?: number
    daily?: DailyUsage[]
  }
  worktime?: {
    totalDurationSec?: number
    daily?: DailyWork[]
    dailySegmented?: DailySegmentedWork[]
    projects?: ProjectWork[]
  }
  summaryCost?: {
    count?: number
  }
  completions?: {
    daily?: DailyCompletion[]
  }
}

export interface DailyUsage {
  date: string
  avgFiveHourUtil: number
  maxFiveHourUtil: number
}

export interface DailyWork {
  date: string
  workSec: number
}

export interface DailySegmentedWork {
  date: string
  segments: number[] // seconds per time segment
}

export interface DailyCompletion {
  date: string
  segments: number[] // 5 time segments: [00-05, 05-10, 10-15, 15-20, 20-24]
}

export interface ProjectWork {
  projectPath: string
  totalDurationSec: number
}
