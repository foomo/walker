import React from "react";
import { styled } from "../theme";
import { ServiceClient } from "../service/service";
import { getAsyncClient } from "../service/utils";
import { ScrapeResult, FilterOptions } from "../service/vo-walker";

const Page = styled.div`
  padding: 2rem;
  font-family: sans-serif;
  a, input, th, td, button {
    font-size: 1rem;
  }
  input, button {
    border: 1px solid grey;
    border-radius: none;
    padding: 1rem;
  }
  button {
    background-color: grey;
  }
  table, tr, td {
    border-collapse: collapse;
    width: 100%;
  }
`;


const ScanResultRow = styled.tr`
  &:nth-child(even) {
    background-color: grey;
  }
  td {
    padding: 0.5rem;
    text-overflow: ellipsis;
    overflow: hidden;
  }
`;

const StatusList = styled.ul`
  margin: 0;
`;

const StatusListItem = styled.ul`
  margin: .5rem;
  padding: .5rem;
  list-style: none;
  display: inline-block;
  border: ${props => (props.selected ? "1px solid red" : "")};
  background-color: green;
  color: white;
`;

const Pager = styled.div`
  border: 1px solid red;
  width: 100%;
  display: flex;
  justify-content: space-between;
`;
const PagerButton = styled.button`
  flex-direction: row;
  border: 1px solid black;
`;

interface IndexProps {}

interface State {
  prefix: string;
  page: number;
  numPages: number;
  pageSize:number;
  status: number[];
  results: ScrapeResult[];
  filterOptions: FilterOptions;
}

class Index extends React.Component<IndexProps, State> {
  private client: ServiceClient;
  constructor(props: IndexProps) {
    super(props);
    this.state = {
      filterOptions: {
        Status: [],
        MinDur: 0,
        MaxDur: 0
      },
      page: 0,
      numPages: 0,
      pageSize:100,
      status: [],
      results: [],
      prefix: ""
    };
    this.client = getAsyncClient(ServiceClient);
  }
  getResults = () => {
    this.client
      .getResults(
        {
          Prefix: this.state.prefix,
          Status: this.state.status,
          MinDur: 0,
          MaxDur: 0
        },
        this.state.page,
        this.state.pageSize,        
      )
      .then(response => {
        console.log(response);
        this.setState({
          results: response.results,
          filterOptions: response.filterOptions,
          numPages: response.numPages
        });
      });
  };
  page = (inc:number) => {
    this.setState({page:this.state.page+inc});
    this.getResults()
  }
  render() {
    return (
      <Page>
        Hello
        <form
          onSubmit={e => {
            e.preventDefault();
            console.log("time to submit");
          }}
        >
          <input
            type="search"
            placeholder="enter a prefix to filter"
            onChange={e =>
              this.setState({
                prefix: e.target.value
              })
            }
          />
          <StatusList>
            {this.state.filterOptions.Status.sort(
              (a, b) => (a.Code > b.Code ? 1 : -1)
            ).map(s => (
              <StatusListItem
                selected={this.state.status.indexOf(s.Code) > -1}
                key={s.Code}
                onClick={e => {
                  this.setState({ status: [s.Code] });
                }}
              >
                {s.Code} {s.Count}
              </StatusListItem>
            ))}
          </StatusList>
          <button onClick={this.getResults}>load</button>
        </form>
        <Pager>
          <PagerButton onClick={e => this.page(-1)}>Prev</PagerButton>
          {this.state.page} / {this.state.numPages}
          <PagerButton onClick={e => this.page(1)}>Next</PagerButton>
        </Pager>
        <table>
          <thead>
            <tr>
              <th>#</th>
              <th>Code</th>
              <th>Error</th>
              <th>Duration ms</th>
              <th>url</th>
            </tr>
          </thead>
          <tbody>
            {this.state.results.map((r, i) => (
              <ScanResultRow key={i}>
                <td>{this.state.page * this.state.pageSize + i}</td>
                <td>{r.Code}</td>
                <td>{r.Error}</td>
                <td>{Math.round(r.Duration / 10000)}</td>
                <td>{r.TargetURL}</td>
              </ScanResultRow>
            ))}
          </tbody>
        </table>
      </Page>
    );
  }
}

export default Index;
