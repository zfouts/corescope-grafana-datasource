import { DataSourcePlugin } from '@grafana/data';
import { DataSource } from './datasource';
import { ConfigEditor } from './components/ConfigEditor';
import { QueryEditor } from './components/QueryEditor';
import { VariableQueryEditor } from './components/VariableQueryEditor';
import { CoreScopeQuery, CoreScopeOptions, CoreScopeSecureJsonData } from './types';

export const plugin = new DataSourcePlugin<DataSource, CoreScopeQuery, CoreScopeOptions, CoreScopeSecureJsonData>(
  DataSource
)
  .setConfigEditor(ConfigEditor)
  .setQueryEditor(QueryEditor)
  .setVariableQueryEditor(VariableQueryEditor);
