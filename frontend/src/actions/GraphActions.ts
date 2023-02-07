import { ActionType, createAction, createStandardAction } from 'typesafe-actions';
import { CytoscapeEvent, EdgeMode, GraphDefinition, Layout, NodeParamsType, RankResult } from '../types/Graph';
import { ActionKeys } from './ActionKeys';
import { TimeInMilliseconds } from 'types/Common';

export const GraphActions = {
  onNamespaceChange: createAction(ActionKeys.GRAPH_ON_NAMESPACE_CHANGE),
  setEdgeMode: createStandardAction(ActionKeys.GRAPH_SET_EDGE_MODE)<EdgeMode>(),
  setGraphDefinition: createStandardAction(ActionKeys.GRAPH_SET_DEFINITION)<GraphDefinition>(),
  setLayout: createStandardAction(ActionKeys.GRAPH_SET_LAYOUT)<Layout>(),
  setNamespaceLayout: createStandardAction(ActionKeys.GRAPH_SET_NAMESPACE_LAYOUT)<Layout>(),
  setNode: createStandardAction(ActionKeys.GRAPH_SET_NODE)<NodeParamsType | undefined>(),
  setRankResult: createStandardAction(ActionKeys.GRAPH_SET_RANK_RESULT)<RankResult>(),
  setUpdateTime: createStandardAction(ActionKeys.GRAPH_SET_UPDATE_TIME)<TimeInMilliseconds>(),
  updateSummary: createStandardAction(ActionKeys.GRAPH_UPDATE_SUMMARY)<CytoscapeEvent>()
};

export type GraphAction = ActionType<typeof GraphActions>;
