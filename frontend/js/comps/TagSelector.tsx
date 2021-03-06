import * as React from 'react';
import {inject, observer} from "mobx-react";
import {
    Area,
    AreaChart,
    Brush,
    CartesianGrid,
    ComposedChart,
    Legend,
    Line,
    LineChart,
    ReferenceLine,
    ResponsiveContainer,
    Tooltip,
    XAxis,
    YAxis
} from 'recharts';
import {SpammerStore} from "../stores/SpammerStore";
import {StyleRulesCallback, Theme} from "@material-ui/core/styles";
import FormControl from '@material-ui/core/FormControl';
import FormHelperText from '@material-ui/core/FormHelperText';
import {TextField, WithStyles} from "@material-ui/core";
import withStyles from "@material-ui/core/styles/withStyles";
import CircularProgress from '@material-ui/core/CircularProgress';

interface Props {
    spammerStore?: SpammerStore;
}

const styles: StyleRulesCallback = (theme: Theme) => ({
    container: {
        display: 'flex',
        flexWrap: 'wrap',
    },
    paper: {
        padding: 16,
    },
    root: {
        flexGrow: 1,
        marginTop: theme.spacing.unit * 2,
    },
    formControl: {
        marginLeft: 10,
    },
    textField: {
        width: 300
    },
    nodeSelect: {
        width: 'auto',
    },
    nodeSelectForm: {
        display: 'inline',
        verticalAlign: 'center',
    },
});

@inject("spammerStore")
@observer
class tagselector extends React.Component<Props & WithStyles, {}> {
    changeTag = (e: any) => {
        this.props.spammerStore.changeTag(e.target.value);
    }

    updateTag = (e: any) => {
        if (e.key !== 'Enter') return true;
        this.props.spammerStore.updateTag();
    }

    render() {
        let classes = this.props.classes;
        let {tag, disable_controls, updating_node, tag_updated} = this.props.spammerStore;
        return (
            <FormControl className={classes.formControl}>
                <TextField
                    disabled={updating_node}
                    id="name"
                    label="Tag"
                    className={classes.textField}
                    value={tag}
                    onChange={this.changeTag}
                    onKeyDown={this.updateTag}
                    margin="normal"
                />
                <FormHelperText style={{fontSize: 14}}>
                    {
                        tag_updated ?
                            <span style={{color: "#40d803"}}>Tag updated.</span>
                            :
                            updating_node ?
                                <span>
                                    Updating...<CircularProgress className="button_loader" size={10}/>
                                </span>
                                :
                                <span style={{color: "#196486"}}>Hit enter in the field to update the tag.</span>
                    }
                </FormHelperText>
            </FormControl>
        );
    }
}

export var TagSelector = withStyles(styles)(tagselector);