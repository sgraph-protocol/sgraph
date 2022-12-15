export * from './Controller'
export * from './Provider'
export * from './Relation'

import { Provider } from './Provider'
import { Relation } from './Relation'
import { Controller } from './Controller'

export const accountProviders = { Provider, Relation, Controller }
